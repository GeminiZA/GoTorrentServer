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

type BlockResponse struct {
	PieceIndex  uint32
	BeginOffset uint32
	Block       []byte
}

func (res *BlockResponse) EqualToReq(req *BlockRequest) bool {
	return res.BeginOffset == uint32(req.Info.BeginOffset) &&
		res.PieceIndex == uint32(req.Info.PieceIndex) &&
		len(res.Block) == int(req.Info.Length)
}

func (resa *BlockResponse) Equal(resb *BlockResponse) bool {
	return resa.BeginOffset == resb.BeginOffset &&
		resa.PieceIndex == resb.PieceIndex &&
		len(resa.Block) == len(resb.Block)
}

type BlockRequest struct {
	Info    bundle.BlockInfo
	ReqTime time.Time
	Sent    bool
}

func (bra *BlockRequest) Equal(brb *BlockRequest) bool {
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
	RequestOutMax         int
	ResponseOutMax        int
	requestInQueue        []*BlockRequest
	requestOutQueue       []*BlockRequest
	responseInQueue       []*BlockResponse
	responseOutQueue      []*BlockResponse
	requestCancelledQueue []*BlockRequest
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
		RequestOutMax:         30,
		ResponseOutMax:        30,
		requestInQueue:        make([]*BlockRequest, 0),
		requestOutQueue:       make([]*BlockRequest, 0),
		responseOutQueue:      make([]*BlockResponse, 0),
		responseInQueue:       make([]*BlockResponse, 0),
		requestCancelledQueue: make([]*BlockRequest, 0),
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

	go peer.run()

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

	peer.ResponseOutMax = newMax
}

func (peer *Peer) SetMaxRequests(newMax int) {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	peer.RequestOutMax = newMax
}

func (peer *Peer) NumRequestsCanAdd() int {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	return peer.RequestOutMax - len(peer.requestOutQueue)
}

func (peer *Peer) NumResponsesCanAdd() int {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	return peer.ResponseOutMax - len(peer.responseOutQueue)
}

func (peer *Peer) QueueRequestOut(bi bundle.BlockInfo) error {
	if len(peer.requestOutQueue) == peer.RequestOutMax {
		return errors.New("requestOutQueue at capacity")
	}

	peer.mux.Lock()
	defer peer.mux.Unlock()

	newReq := &BlockRequest{
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

func (peer *Peer) GetRequestsIn() []*BlockRequest {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	ret := peer.requestInQueue

	return ret
}

func (peer *Peer) QueueResponseOut(res *BlockResponse) error {
	peer.mux.Lock()
	defer peer.mux.Unlock()

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

func (peer *Peer) GetResponsesIn() []*BlockResponse {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	if len(peer.responseInQueue) == 0 {
		return nil
	}
	ret := peer.responseInQueue
	peer.responseInQueue = make([]*BlockResponse, 0)

	return ret
}

func (peer *Peer) GetCancelledRequests() []*BlockRequest {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	if len(peer.requestCancelledQueue) == 0 {
		return nil
	}

	ret := peer.requestCancelledQueue
	peer.requestCancelledQueue = make([]*BlockRequest, 0)

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

	newReq := &BlockRequest{
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
	return peer.bitField.GetBit(pieceIndex)
}

func (peer *Peer) Have(pieceIndex int64) error {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	if !peer.HasPiece(pieceIndex) {
		peer.sendHave(uint32(pieceIndex))
	}
	return nil
}

func (peer *Peer) run() {
	// Read bitfield

	const BLOCK_REQUEST_TIMEOUT_MS = 5000
	for peer.Connected {
		peer.UpdateRates()

		var err error

		// Check if alive

		if time.Now().After(peer.timeLastReceived.Add(30 * time.Second)) {
			// Cancel all requests
			peer.requestCancelledQueue = append(peer.requestCancelledQueue, peer.requestOutQueue...)
			peer.requestOutQueue = make([]*BlockRequest, 0)
			peer.Close()
			break
		}

		// If client has all pieces of remote peer no longer interested
		if peer.bitField != nil &&
			peer.remoteBitfield != nil &&
			peer.AmInterested &&
			peer.bitField.HasAll(peer.remoteBitfield) {
			peer.sendNotInterested()
		}

		if debugopts.PEER_DEBUG {
			fmt.Println("Requests out queue:")
			for _, req := range peer.requestOutQueue {
				fmt.Printf("(%d, %d, %d) (sent: %v)\n", req.Info.PieceIndex, req.Info.BeginOffset, req.Info.Length, req.Sent)
			}
			fmt.Println("Reading messages in...")
		}

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

		if debugopts.PEER_DEBUG {
			fmt.Println("Done reading messages in...")
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

		// Keep alive
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
	}
}

func (peer *Peer) UpdateRates() {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	now := time.Now()
	downloadElapsed := now.Sub(peer.lastDownloadUpdate).Milliseconds()
	uploadElapsed := now.Sub(peer.lastUploadUpdate).Milliseconds()
	if downloadElapsed > 1000 && peer.bytesDownloaded > 0 {
		peer.DownloadRateKB = float64(peer.bytesDownloaded) / (float64(downloadElapsed) / 1000) / 1024
		peer.TotalBytesDownloaded += peer.bytesDownloaded
		peer.bytesDownloaded = 0
		peer.lastDownloadUpdate = now
	}
	if uploadElapsed > 1000 && peer.bytesUploaded > 0 {
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
		fmt.Printf("Handshake successfully read from peer(%s)...\n", handshakeMsg.PeerID)
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
	if peer.bitField != nil && peer.bitField.NumSet > 0 {
		err = peer.sendBitField()
		if err != nil {
			return err
		}
	}
	if debugopts.PEER_DEBUG {
		fmt.Printf("Sent handshake to peer(%s)...\n", peer.RemotePeerID)
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
		peer.requestInQueue = make([]*BlockRequest, 0)
	case message.HAVE:
		peer.remoteBitfield.SetBit(int64(msg.Index))
	case message.BITFIELD:
		peer.remoteBitfield = bitfield.FromBytes(msg.BitField, peer.numPieces)
	case message.REQUEST:
		if !peer.RemoteInterested || peer.AmChoking {
			return nil
		}
		newReq := &BlockRequest{
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
		newRes := &BlockResponse{
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
		req := &BlockRequest{
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
		fmt.Printf("Sending msg to peer(%s): ", peer.RemotePeerID)
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

func (peer *Peer) sendRequest(req *BlockRequest) error {
	return peer.sendMessage(message.NewRequest(&req.Info))
}

func (peer *Peer) sendPiece(res *BlockResponse) error {
	found := false
	for _, req := range peer.requestInQueue {
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

func (peer *Peer) sendCancel(req *BlockRequest) error {
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
	const MESSAGE_READ_TIMEOUT_MS = 1000
	const MAX_MESSAGE_LENGTH = 17 * 1024 // 17kb for 16 kb max piece length
	msgLenBytes := make([]byte, 4)
	peer.conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond)) // 20 ms read for message length
	n, err := peer.conn.Read(msgLenBytes)
	if err != nil {
		return nil, err
	}
	if n != 4 {
		return nil, fmt.Errorf("malformed message length read: 0x%x", msgLenBytes)
	}
	msgLen := int(binary.BigEndian.Uint32(msgLenBytes))
	if msgLen > MAX_MESSAGE_LENGTH {
		return nil, errors.New("message length exceeds max of 17kb")
	}
	if debugopts.PEER_DEBUG {
		fmt.Printf("Message Length read: %d (0x%x)\n", msgLen, msgLenBytes)
	}
	if msgLen == 0 {
		return message.NewKeepAlive(), nil
	}
	peer.conn.SetReadDeadline(time.Now().Add(MESSAGE_READ_TIMEOUT_MS * time.Millisecond))
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
	fmt.Printf("Peer (%s) Got message: ", peer.RemotePeerID)
	msg.Print()
	return msg, nil
}
