// Todo:
// handle incoming peer connections
// Change queues to be thread safe or just use channels

package peer

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/debugopts"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer/message"
)

type blockResponse struct {
	PieceIndex  uint32
	BeginOffset uint32
	Block       []byte
}

func (res *blockResponse) EqualToReq(req *blockRequest) bool {
	return res.BeginOffset == uint32(req.Info.BeginOffset) &&
		res.PieceIndex == uint32(req.Info.PieceIndex) &&
		len(res.Block) == int(req.Info.Length)
}

func (resa *blockResponse) Equal(resb *blockResponse) bool {
	return resa.BeginOffset == resb.BeginOffset &&
		resa.PieceIndex == resb.PieceIndex &&
		len(resa.Block) == len(resb.Block)
}

type blockRequest struct {
	Info    bundle.BlockInfo
	ReqTime time.Time
	Sent    bool
}

func (bra *blockRequest) Equal(brb *blockRequest) bool {
	return bra.Info.BeginOffset == brb.Info.BeginOffset &&
		bra.Info.PieceIndex == brb.Info.PieceIndex &&
		bra.Info.Length == brb.Info.Length
}

type Peer struct {
	// Info
	RemotePeerID   string
	RemoteIP       string
	RemotePort     int
	peerID         string
	infoHash       []byte
	bitField       *bitfield.Bitfield
	remoteBitfield *bitfield.Bitfield
	numPieces      int64
	// Functional
	conn             net.Conn
	timeLastReceived time.Time
	timeLastSent     time.Time
	mux              sync.Mutex
	Connected        bool
	Keepalive        bool
	// State
	AmInterested     bool
	AmChoking        bool
	RemoteInterested bool
	RemoteChoking    bool
	// Queues
	RequestsOutMax        int
	ResponsesOutMax       int
	requestInQueue        []*blockRequest
	pendingRequestInQueue []*blockRequest
	requestOutQueue       []*blockRequest
	responseInQueue       []*blockResponse
	responseOutQueue      []*blockResponse
	requestCancelledQueue []*blockRequest
	// RateKB
	TotalBytesUploaded   int64
	TotalBytesDownloaded int64
	bytesDownloaded      int64
	bytesUploaded        int64
	DownloadRateKB       float64
	UploadRateKB         float64
	lastDownloadUpdate   time.Time
	lastUploadUpdate     time.Time

	TimeConnected time.Time
}

func Connect(
	remotePeerID string,
	remoteIP string,
	remotePort int,
	infohash []byte,
	peerID string,
	myBitfield *bitfield.Bitfield,
	numPieces int64,
) (*Peer, error) {
	peer := Peer{
		peerID:           peerID,
		RemotePeerID:     remotePeerID,
		RemoteIP:         remoteIP,
		RemotePort:       remotePort,
		infoHash:         infohash,
		bitField:         myBitfield,
		remoteBitfield:   bitfield.New(myBitfield.Len()),
		numPieces:        numPieces,
		timeLastReceived: time.Now(),
		timeLastSent:     time.Now(),
		Connected:        false,
		Keepalive:        false,
		AmInterested:     false,
		AmChoking:        true,
		RemoteInterested: false,
		RemoteChoking:    true,
		// Queues
		RequestsOutMax:        10,
		ResponsesOutMax:       10,
		requestInQueue:        make([]*blockRequest, 0),
		requestOutQueue:       make([]*blockRequest, 0),
		responseOutQueue:      make([]*blockResponse, 0),
		responseInQueue:       make([]*blockResponse, 0),
		requestCancelledQueue: make([]*blockRequest, 0),
		// RateKBs
		lastDownloadUpdate:   time.Now(),
		lastUploadUpdate:     time.Now(),
		bytesUploaded:        0,
		bytesDownloaded:      0,
		TotalBytesUploaded:   0,
		TotalBytesDownloaded: 0,
		DownloadRateKB:       0,
		UploadRateKB:         0,
	}

	d := net.Dialer{Timeout: time.Millisecond * 2000}
	conn, err := d.Dial("tcp", fmt.Sprintf("%s:%d", remoteIP, remotePort))
	if err != nil {
		return nil, err
	}

	peer.conn = conn

	err = peer.sendHandshake()
	if err != nil {
		return nil, err
	}

	err = peer.readHandshake()
	if err != nil {
		peer.conn.Close()
		peer.Connected = false
		return nil, err
	}

	peer.Keepalive = true
	peer.Connected = true
	peer.TimeConnected = time.Now()
	peer.conn.SetReadDeadline(time.Time{})

	go peer.runIncoming()
	go peer.runOutgoing()

	return &peer, nil
}

func (peer *Peer) Close() {
	peer.Keepalive = false
	peer.Connected = false
	peer.conn.Close()
}

// Exported Interface

func (peer *Peer) NumResponsesOut() int {
	return len(peer.responseOutQueue)
}

func (peer *Peer) SetMaxResponses(newMax int) {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	peer.ResponsesOutMax = newMax
}

func (peer *Peer) SetMaxRequests(newMax int) {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	peer.RequestsOutMax = newMax
}

func (peer *Peer) NumRequestsCanAdd() int {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	return peer.RequestsOutMax - len(peer.requestOutQueue)
}

func (peer *Peer) NumResponsesCanAdd() int {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	return peer.ResponsesOutMax - len(peer.responseOutQueue)
}

func (peer *Peer) QueueRequestOut(bi bundle.BlockInfo) error {
	if len(peer.requestOutQueue) == peer.RequestsOutMax {
		return errors.New("requestOutQueue at capacity")
	}

	peer.mux.Lock()
	defer peer.mux.Unlock()

	newReq := &blockRequest{
		Info:    bi,
		Sent:    false,
		ReqTime: time.Time{},
	}
	for _, req := range peer.requestOutQueue {
		if req.Equal(newReq) {
			return errors.New("block already in queue")
		}
	}
	peer.requestOutQueue = append(peer.requestOutQueue, newReq)
	return nil
}

func (peer *Peer) GetRequestsIn(n int) []*blockRequest {
	peer.mux.Lock()
	defer peer.mux.Unlock()
	ret := peer.requestInQueue[:n]

	peer.requestInQueue = peer.requestInQueue[n:]
	peer.pendingRequestInQueue = append(peer.pendingRequestInQueue, ret...)

	return ret
}

func (peer *Peer) QueueResponseOut(pieceIndex uint32, beginOffest uint32, block []byte) error {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	res := &blockResponse{
		PieceIndex:  pieceIndex,
		BeginOffset: beginOffest,
		Block:       block,
	}

	exists := false
	for _, req := range peer.requestInQueue {
		if res.EqualToReq(req) {
			exists = true
			break
		}
	}
	if !exists {
		// Dont return a error because the piece may have been cancelled which is expected behavior
		return nil
	}

	for _, res := range peer.responseOutQueue {
		if res.Equal(res) {
			return errors.New("response already in queue")
		}
	}

	peer.responseOutQueue = append(peer.responseOutQueue, res)
	return nil
}

func (peer *Peer) NumResponsesIn() int {
	return len(peer.responseInQueue)
}

func (peer *Peer) NumRequestsIn() int {
	return len(peer.requestInQueue)
}

func (peer *Peer) NumRequestsCancelled() int {
	return len(peer.requestCancelledQueue)
}

func (peer *Peer) GetResponsesIn() []*blockResponse {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	if len(peer.responseInQueue) == 0 {
		return nil
	}
	ret := peer.responseInQueue
	peer.responseInQueue = make([]*blockResponse, 0)

	return ret
}

func (peer *Peer) GetCancelledRequests() []*blockRequest {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	if len(peer.requestCancelledQueue) == 0 {
		return nil
	}

	ret := peer.requestCancelledQueue
	peer.requestCancelledQueue = make([]*blockRequest, 0)

	return ret
}

func (peer *Peer) Choke() error {
	peer.mux.Lock()
	defer peer.mux.Unlock()
	if !peer.Connected || peer.AmChoking {
		return nil
	}
	err := peer.sendChoke()
	return err
}

func (peer *Peer) Unchoke() error {
	peer.mux.Lock()
	defer peer.mux.Unlock()
	if !peer.Connected || !peer.AmChoking {
		return nil
	}
	err := peer.sendUnchoke()
	return err
}

func (peer *Peer) CancelRequest(bi *bundle.BlockInfo) error {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	newReq := &blockRequest{
		Info:    *bi,
		Sent:    false,
		ReqTime: time.Time{},
	}

	return peer.sendCancel(newReq)
}

func (peer *Peer) HasPiece(pieceIndex int64) bool {
	if peer.bitField == nil {
		return false
	}
	return peer.remoteBitfield.GetBit(pieceIndex)
}

func (peer *Peer) Have(pieceIndex int64) error {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	if !peer.HasPiece(pieceIndex) {
		peer.sendHave(uint32(pieceIndex))
	}
	return nil
}

func (peer *Peer) runOutgoing() {
	const BLOCK_REQUEST_TIMEOUT_MS = 5000
	var err error
	for peer.Connected {
		peer.UpdateRates()

		// Check if alive
		if time.Now().After(peer.timeLastReceived.Add(30 * time.Second)) {
			// Cancel all requests
			peer.requestCancelledQueue = append(peer.requestCancelledQueue, peer.requestOutQueue...)
			peer.requestOutQueue = make([]*blockRequest, 0)
			peer.Close()
			break
		} // end check if alive

		// If client has all pieces of remote peer no longer interested
		if peer.bitField != nil &&
			peer.remoteBitfield != nil &&
			peer.AmInterested &&
			peer.bitField.HasAll(peer.remoteBitfield) {
			peer.sendNotInterested()
		}

		// Process messages out
		if len(peer.requestOutQueue) > 0 {
			if !peer.AmInterested {
				peer.sendInterested()
			} else {
				if !peer.RemoteChoking {
					for i := range peer.requestOutQueue {
						if !peer.requestOutQueue[i].Sent {
							err = peer.sendRequest(peer.requestOutQueue[i])
							if err != nil {
								fmt.Println("Error sending request to peer: ", err)
							}
							peer.requestOutQueue[i].Sent = true
							peer.requestOutQueue[i].ReqTime = time.Now()
						}
					}
				}
			}
		}
		if debugopts.PEER_DEBUG {
			fmt.Println("Done sending requests out...")
		}

		if len(peer.responseOutQueue) > 0 {
			if !peer.AmChoking {
				for len(peer.responseOutQueue) > 0 {
					err = peer.sendPiece(peer.responseOutQueue[0])
					if err != nil {
						fmt.Println("Error sending piece to peer: ", err)
					}
					peer.responseOutQueue = peer.responseOutQueue[1:]
				}
			}
		}
		if debugopts.PEER_DEBUG {
			fmt.Println("Done sending responses out...")
		}

		// Send keep alive
		if time.Now().After(peer.timeLastSent.Add(20 * time.Second)) {
			err = peer.sendKeepAlive()
			if err != nil {
				fmt.Println("Error sending keep alive: ", err)
			}
		}

		// Handle timed pieces
		i := 0
		for i < len(peer.requestOutQueue) {
			if peer.requestOutQueue[i].Sent && time.Now().After(peer.requestOutQueue[i].ReqTime.Add(BLOCK_REQUEST_TIMEOUT_MS*time.Millisecond)) {
				peer.requestCancelledQueue = append(peer.requestCancelledQueue, peer.requestOutQueue[i])
				peer.requestOutQueue = append(peer.requestOutQueue[:i], peer.requestOutQueue[i+1:]...)
				continue
			}
			i++
		}

		if debugopts.PEER_DEBUG {
			fmt.Println("Done handling timed pieces...")
		}

		time.Sleep(100 * time.Millisecond)

	}
}

func (peer *Peer) runIncoming() {
	for peer.Connected {
		// Process messages in
		for numReads := 0; numReads < 15; numReads++ {
			msgIn, err := peer.readMessage()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					break
					// fmt.Println("No new message")
				} else {
					fmt.Println("Error reading message from peer: ", err)
				}
			} else {
				err = peer.handleMessageIn(msgIn)
				if err != nil {
					fmt.Println("Error handling message from peer: ", err)
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (peer *Peer) UpdateRates() {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	now := time.Now()
	downloadElapsed := now.Sub(peer.lastDownloadUpdate).Milliseconds()
	uploadElapsed := now.Sub(peer.lastUploadUpdate).Milliseconds()
	if downloadElapsed > 1000 {
		peer.DownloadRateKB = float64(peer.bytesDownloaded) / (float64(downloadElapsed) / 1000) / 1024
		peer.TotalBytesDownloaded += peer.bytesDownloaded
		peer.bytesDownloaded = 0
		peer.lastDownloadUpdate = now
	}
	if uploadElapsed > 1000 {
		peer.UploadRateKB = float64(peer.bytesUploaded) / (float64(uploadElapsed) / 1000) / 1024
		peer.TotalBytesUploaded += peer.bytesUploaded
		peer.bytesUploaded = 0
		peer.lastUploadUpdate = now
	}
}

// handshake interface

func (peer *Peer) readHandshake() error {
	const HANDSHAKE_TIMEOUT_MS = 5000
	peer.conn.SetReadDeadline(time.Now().Add(HANDSHAKE_TIMEOUT_MS * time.Millisecond))
	handshakeBytes := make([]byte, 68)
	n, err := peer.conn.Read(handshakeBytes)
	if err != nil {
		return err
	}
	if n != 68 {
		return errors.New("invalid handshake length")
	}
	if debugopts.PEER_DEBUG {
		fmt.Printf("Read handshake bytes from peer: %x\n", handshakeBytes)
	}
	handshakeMsg, err := message.ParseHandshake(handshakeBytes)
	if err != nil {
		return err
	}

	if debugopts.PEER_DEBUG {
		fmt.Printf("Handshake successfully read from peer(%s)...\n", peer.RemoteIP)
	}
	peer.RemotePeerID = handshakeMsg.PeerID
	//	if peer.RemotePeerID != handshakeMsg.PeerID {
	//return errors.New("peerID mismatch")
	//}
	if !bytes.Equal(peer.infoHash, handshakeMsg.InfoHash) {
		return errors.New("infohash mismatch")
	}
	return nil
}

func (peer *Peer) sendHandshake() error {
	const HANDSHAKE_TIMEOUT_MS = 5000
	peer.conn.SetWriteDeadline(time.Now().Add(HANDSHAKE_TIMEOUT_MS * time.Millisecond))
	handshakeMsg := message.NewHandshake(peer.infoHash, peer.peerID)
	n, err := peer.conn.Write(handshakeMsg.GetBytes())
	if err != nil {
		return err
	}
	if n != 68 {
		return errors.New("handshake send incomplete")
	}
	if debugopts.PEER_DEBUG {
		fmt.Printf("Sent handshake to peer(%s)...\n", peer.RemoteIP)
	}
	if peer.bitField != nil && peer.bitField.NumSet > 0 {
		err = peer.sendBitField()
		if err != nil {
			return err
		}
	}
	return nil
}

func (peer *Peer) handleMessageIn(msg *message.Message) error {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	peer.timeLastReceived = time.Now()
	if msg == nil {
		return nil
	}
	switch msg.Type {
	case message.KEEP_ALIVE:
		// Do nothing
	case message.CHOKE:
		peer.RemoteChoking = true
	case message.UNCHOKE:
		peer.RemoteChoking = false
	case message.INTERESTED:
		peer.RemoteInterested = true
	case message.NOT_INTERESTED:
		peer.RemoteInterested = false
		peer.requestInQueue = make([]*blockRequest, 0)
	case message.HAVE:
		peer.remoteBitfield.SetBit(int64(msg.Index))
	case message.BITFIELD:
		peer.remoteBitfield = bitfield.FromBytes(msg.BitField, peer.numPieces)
	case message.REQUEST:
		if !peer.RemoteInterested || peer.AmChoking {
			return nil
		}
		newReq := &blockRequest{
			Info: bundle.BlockInfo{
				PieceIndex:  int64(msg.Index),
				BeginOffset: int64(msg.Begin),
				Length:      int64(msg.Length),
			},
			ReqTime: time.Now(),
			Sent:    false,
		}
		// Move to back of queue
		i := 0
		for i < len(peer.requestInQueue) {
			if peer.requestInQueue[i].Equal(newReq) {
				peer.requestInQueue = append(peer.requestInQueue[:i], peer.requestInQueue[i+1:]...)
				break
			}
			i++
		}
		peer.requestInQueue = append(peer.requestInQueue, newReq)
	case message.PIECE:
		peer.bytesDownloaded += int64(len(msg.Piece))
		newRes := &blockResponse{
			PieceIndex:  msg.Index,
			BeginOffset: msg.Begin,
			Block:       msg.Piece,
		}
		i := 0
		found := false
		for i < len(peer.requestOutQueue) {
			if newRes.EqualToReq(peer.requestOutQueue[i]) {
				found = true
				// Remove from out queue
				peer.requestOutQueue = append(peer.requestOutQueue[:i], peer.requestOutQueue[i+1:]...)
				break
			}
			i++
		}
		if !found {
			return nil
		}
		peer.responseInQueue = append(peer.responseInQueue, newRes)
		fmt.Printf("Response added to responseInQueue\n")
	case message.CANCEL:
		req := &blockRequest{
			Info: bundle.BlockInfo{
				PieceIndex:  int64(msg.Index),
				BeginOffset: int64(msg.Begin),
				Length:      int64(msg.Length),
			},
			Sent:    false,
			ReqTime: time.Time{},
		}
		i := 0
		for i < len(peer.requestInQueue) {
			if peer.requestInQueue[i].Equal(req) {
				peer.requestInQueue = append(peer.requestInQueue[:i], peer.requestInQueue[i+1:]...)
				break
			}
			i++
		}
		for i < len(peer.pendingRequestInQueue) {
			if peer.pendingRequestInQueue[i].Equal(req) {
				peer.pendingRequestInQueue = append(peer.pendingRequestInQueue[:i], peer.pendingRequestInQueue[i+1:]...)
				break
			}
			i++
		}
	case message.PORT:
		// Not implemented
	default:
		return errors.New("unknown message type")
	}
	return nil
}

// Connection interface

func (peer *Peer) sendMessage(msg *message.Message) error {
	if !peer.Connected {
		return nil
	}
	const MESSAGE_SEND_TIMEOUT = 1000
	peer.conn.SetWriteDeadline(time.Now().Add(MESSAGE_SEND_TIMEOUT * time.Millisecond))
	if debugopts.PEER_DEBUG {
		fmt.Printf("Sending msg to peer(%s): ", peer.RemoteIP)
		msg.Print()
	}
	msgBytes := msg.GetBytes()
	n, err := peer.conn.Write(msgBytes)
	if err != nil {
		return err
	}
	if n != len(msgBytes) {
		return errors.New("incomplete message send")
	}
	peer.timeLastSent = time.Now()
	return nil
}

func (peer *Peer) sendKeepAlive() error {
	return peer.sendMessage(message.NewKeepAlive())
}

func (peer *Peer) sendInterested() error {
	peer.AmInterested = true
	return peer.sendMessage(message.NewInterested())
}

func (peer *Peer) sendNotInterested() error {
	peer.AmInterested = false
	return peer.sendMessage(message.NewNotInterested())
}

func (peer *Peer) sendChoke() error {
	peer.AmChoking = true
	return peer.sendMessage(message.NewChoke())
}

func (peer *Peer) sendUnchoke() error {
	peer.AmChoking = false
	return peer.sendMessage(message.NewUnchoke())
}

func (peer *Peer) sendHave(pieceIndex uint32) error {
	if peer.bitField.GetBit(int64(pieceIndex)) {
		return nil
	}
	return peer.sendMessage(message.NewHave(pieceIndex))
}

func (peer *Peer) sendBitField() error {
	return peer.sendMessage(message.NewBitfield(peer.bitField.Bytes))
}

func (peer *Peer) sendRequest(req *blockRequest) error {
	return peer.sendMessage(message.NewRequest(&req.Info))
}

func (peer *Peer) sendPiece(res *blockResponse) error {
	found := false
	for _, req := range peer.pendingRequestInQueue {
		if res.EqualToReq(req) {
			found = true
			break
		}
	}
	// Piece probably cancelled
	if !found {
		return nil
	}
	peer.bytesUploaded += int64(len(res.Block))
	return peer.sendMessage(message.NewPiece(res.PieceIndex, res.BeginOffset, res.Block))
}

func (peer *Peer) sendCancel(req *blockRequest) error {
	i := 0
	found := false
	for i < len(peer.requestOutQueue) {
		if peer.requestOutQueue[i].Equal(req) {
			peer.requestOutQueue = append(peer.requestOutQueue[:i], peer.requestOutQueue[i+1:]...)
			found = true
			break
		}
		i++
	}
	if !found {
		return nil
	}
	return peer.sendMessage(message.NewCancel(&req.Info))
}

func (peer *Peer) readMessage() (*message.Message, error) {
	if !peer.Connected {
		return nil, nil
	}
	const MESSAGE_READ_TIMEOUT_MS = 2000
	const MAX_MESSAGE_LENGTH = 17 * 1024 // 17kb for 16 kb max piece length
	// peer.conn.SetReadDeadline(time.Time{})
	msgLenBytes := make([]byte, 4)
	n, err := peer.conn.Read(msgLenBytes)
	if err != nil {
		return nil, err
	}
	if n != 4 {
		return nil, fmt.Errorf("malformed message length read(length %d): 0x%x", n, msgLenBytes)
	}
	msgLen := int(binary.BigEndian.Uint32(msgLenBytes))
	if msgLen > MAX_MESSAGE_LENGTH {
		fmt.Printf("%x\n", msgLenBytes)
		panic("message length exceeds max")
		// return nil, errors.New("message length exceeds max of 17kb")
	}
	if debugopts.PEER_DEBUG {
		fmt.Printf("Message Length read: %d (0x%x)\n", msgLen, msgLenBytes)
	}
	if msgLen == 0 {
		return message.NewKeepAlive(), nil
	}
	// peer.conn.SetReadDeadline(time.Now().Add(MESSAGE_READ_TIMEOUT_MS * time.Millisecond))
	msgBytes := make([]byte, 0)
	bytesRead := 0
	for len(msgBytes) < msgLen {
		buf := make([]byte, (msgLen - bytesRead))
		n, err = peer.conn.Read(buf)
		if err != nil {
			return nil, err
		}
		bytesRead += n
		msgBytes = append(msgBytes, buf[0:n]...)
	}
	// fmt.Printf("Message bytes read: \n %x\n", msgBytes)
	msg, err := message.ParseMessage(msgBytes)
	if err != nil {
		return nil, err
	}
	if debugopts.PEER_DEBUG {
		fmt.Printf("Peer (%s) Got message: ", peer.RemoteIP)
	}
	msg.Print()
	return msg, nil
}
