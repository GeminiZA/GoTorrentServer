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

func (res blockResponse) EqualToReq(req blockRequest) bool {
	return res.BeginOffset == uint32(req.info.BeginOffset) &&
		res.PieceIndex == uint32(req.info.PieceIndex) &&
		len(res.Block) == int(req.info.Length)
}

func (resa blockResponse) Equal(resb blockResponse) bool {
	return resa.BeginOffset == resb.BeginOffset &&
		resa.PieceIndex == resb.PieceIndex &&
		len(resa.Block) == len(resb.Block)
}

type blockRequest struct {
	info    bundle.BlockInfo
	reqTime time.Time
	sent    bool
}

func (bra blockRequest) Equal(brb blockRequest) bool {
	return bra.info.BeginOffset == brb.info.BeginOffset &&
		bra.info.PieceIndex == brb.info.PieceIndex &&
		bra.info.Length == brb.info.Length
}

type Peer struct {
	// Info
	remotePeerID   string
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
	amInterested     bool
	amChoking        bool
	remoteInterested bool
	remoteChoking    bool
	// Queues
	requestInQueue        []blockRequest
	requestOutQueue       []blockRequest
	responseInQueue       []blockResponse
	responseOutQueue      []blockResponse
	requestCancelledQueue []blockRequest
	// RateKB
	TotalBytesUploaded   int64
	TotalBytesDownloaded int64
	bytesDownloaded      int64
	bytesUploaded        int64
	DownloadRateKB       float64
	UploadRateKB         float64
	lastDownloadUpdate   time.Time
	lastUploadUpdate     time.Time
}

func Connect(
	remotePeerID string,
	remoteIP string,
	remotePort int,
	infohash []byte,
	peerID string,
	bitfield *bitfield.Bitfield,
	numPieces int64,
) (*Peer, error) {
	peer := Peer{
		peerID:           peerID,
		remotePeerID:     remotePeerID,
		infoHash:         infohash,
		bitField:         bitfield,
		remoteBitfield:   nil,
		numPieces:        numPieces,
		timeLastReceived: time.Now(),
		timeLastSent:     time.Now(),
		Connected:        false,
		Keepalive:        false,
		amInterested:     false,
		amChoking:        true,
		remoteInterested: false,
		remoteChoking:    true,
		// Queues
		requestInQueue:        make([]blockRequest, 0),
		requestOutQueue:       make([]blockRequest, 0),
		responseOutQueue:      make([]blockResponse, 0),
		responseInQueue:       make([]blockResponse, 0),
		requestCancelledQueue: make([]blockRequest, 0),
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

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", remoteIP, remotePort))
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
		return nil, err
	}

	peer.Keepalive = true
	peer.Connected = true

	go peer.run()

	return &peer, nil
}

func (peer *Peer) Close() {
	peer.Keepalive = false
	peer.Connected = false
	peer.conn.Close()
}

// Exported Interface

func (peer *Peer) QueueRequestOut(bi bundle.BlockInfo) error {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	newReq := blockRequest{
		info:    bi,
		sent:    false,
		reqTime: time.Time{},
	}
	for _, req := range peer.requestOutQueue {
		if req.Equal(newReq) {
			return errors.New("block already in queue")
		}
	}
	peer.requestOutQueue = append(peer.requestOutQueue, newReq)
	return nil
}

func (peer *Peer) GetRequestIn() *blockRequest {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	for _, req := range peer.requestInQueue {
		if !req.sent {
			return &req
		}
	}
	return nil
}

func (peer *Peer) QueueResponseOut(pieceIndex int64, beginOffset int64, block []byte) error {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	newRes := blockResponse{
		PieceIndex:  uint32(pieceIndex),
		BeginOffset: uint32(beginOffset),
		Block:       block,
	}

	exists := false
	for _, req := range peer.requestInQueue {
		if newRes.EqualToReq(req) {
			exists = true
			break
		}
	}
	if !exists {
		// Dont return a error because the piece may have been cancelled which is expected behavior
		return nil
	}

	for _, res := range peer.responseOutQueue {
		if res.Equal(newRes) {
			return errors.New("response already in queue")
		}
	}

	peer.responseOutQueue = append(peer.responseOutQueue, newRes)
	return nil
}

func (peer *Peer) GetResponseIn() *blockResponse {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	if len(peer.responseInQueue) == 0 {
		return nil
	}
	br := peer.responseInQueue[0]
	peer.responseInQueue = peer.responseInQueue[1:]

	return &br
}

func (peer *Peer) GetCancelledBlock() *blockRequest {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	if len(peer.requestCancelledQueue) == 0 {
		return nil
	}

	br := peer.requestCancelledQueue[0]
	peer.requestCancelledQueue = peer.requestCancelledQueue[1:]

	return &br
}

func (peer *Peer) Choke() error {
	peer.mux.Lock()
	defer peer.mux.Unlock()
	if !peer.Connected || peer.amChoking {
		return nil
	}
	err := peer.sendChoke()
	return err
}

func (peer *Peer) Unchoke() error {
	peer.mux.Lock()
	defer peer.mux.Unlock()
	if !peer.Connected || !peer.amChoking {
		return nil
	}
	err := peer.sendUnchoke()
	return err
}

func (peer *Peer) CancelRequest(bi *bundle.BlockInfo) error {
	peer.mux.Lock()
	defer peer.mux.Unlock()

	newReq := blockRequest{
		info:    *bi,
		sent:    false,
		reqTime: time.Time{},
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

		// Check if alive

		if time.Now().After(peer.timeLastReceived.Add(30 * time.Second)) {
			// Cancel all requests
			peer.requestCancelledQueue = append(peer.requestCancelledQueue, peer.requestOutQueue...)
			peer.requestOutQueue = make([]blockRequest, 0)
			peer.Close()
			break
		}

		//if len(peer.requestOutQueue) == 0 {
		//peer.sendNotInterested()
		//}

		// Process messages in
		msgIn, err := peer.readMessage()
		if err != nil {
			fmt.Println("Error reading message from peer: ", err)
		} else {
			err = peer.handleMessageIn(msgIn)
			if err != nil {
				fmt.Println("Error handling message from peer: ", err)
			}
		}

		// Process messages out
		if len(peer.requestOutQueue) > 0 {
			if !peer.amInterested {
				peer.sendInterested()
			} else {
				if !peer.remoteChoking {
					for i := range peer.requestOutQueue {
						if !peer.requestOutQueue[i].sent {
							err = peer.sendRequest(peer.requestOutQueue[i])
							if err != nil {
								fmt.Println("Error sending request to peer: ", err)
							}
						}
					}
				}
			}
		}

		if len(peer.responseOutQueue) > 0 {
			if !peer.amChoking {
				for len(peer.responseOutQueue) > 0 {
					err = peer.sendPiece(peer.responseOutQueue[0])
					if err != nil {
						fmt.Println("Error sending piece to peer: ", err)
					}
					peer.responseOutQueue = peer.responseOutQueue[1:]
				}
			}
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
			if peer.requestOutQueue[i].sent && time.Now().After(peer.requestOutQueue[i].reqTime.Add(BLOCK_REQUEST_TIMEOUT_MS*time.Millisecond)) {
				peer.requestCancelledQueue = append(peer.requestCancelledQueue, peer.requestOutQueue[i])
				peer.requestOutQueue = append(peer.requestOutQueue[:i], peer.requestOutQueue[i+1:]...)
				continue
			}
			i++
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
	handshakeMsg, err := message.ParseHandshake(handshakeBytes)
	if err != nil {
		return err
	}
	if debugopts.PEER_DEBUG {
		fmt.Printf("Handshake complete, remoteID: %s\n", handshakeMsg.PeerID)
	}
	if peer.remotePeerID != handshakeMsg.PeerID {
		return errors.New("peerID mismatch")
	}
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
	return nil
}

func (peer *Peer) handleMessageIn(msg *message.Message) error {
	peer.timeLastReceived = time.Now()
	switch msg.Type {
	case message.KEEP_ALIVE:
		// Do nothing
	case message.CHOKE:
		peer.remoteChoking = true
	case message.UNCHOKE:
		peer.remoteChoking = false
	case message.INTERESTED:
		peer.remoteInterested = true
	case message.NOT_INTERESTED:
		peer.mux.Lock()
		peer.remoteInterested = false
		peer.requestInQueue = make([]blockRequest, 0)
		peer.mux.Unlock()
	case message.HAVE:
		peer.remoteBitfield.SetBit(int64(msg.Index))
	case message.BITFIELD:
		peer.remoteBitfield = bitfield.FromBytes(msg.BitField, peer.numPieces)
	case message.REQUEST:
		if !peer.remoteInterested || peer.amChoking {
			return nil
		}
		peer.mux.Lock()
		newReq := blockRequest{
			info: bundle.BlockInfo{
				PieceIndex:  int64(msg.Index),
				BeginOffset: int64(msg.Begin),
				Length:      int64(msg.Length),
			},
			reqTime: time.Now(),
			sent:    false,
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
		peer.mux.Unlock()
	case message.PIECE:
		peer.mux.Lock()
		peer.bytesDownloaded += int64(len(msg.Piece))
		newRes := blockResponse{
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
		peer.mux.Unlock()
	case message.CANCEL:
		peer.mux.Lock()
		req := blockRequest{
			info: bundle.BlockInfo{
				PieceIndex:  int64(msg.Index),
				BeginOffset: int64(msg.Begin),
				Length:      int64(msg.Length),
			},
			sent:    false,
			reqTime: time.Time{},
		}
		i := 0
		for i < len(peer.requestInQueue) {
			if peer.requestInQueue[i].Equal(req) {
				peer.requestInQueue = append(peer.requestInQueue[:i], peer.requestInQueue[i+1:]...)
				break
			}
			i++
		}
		peer.mux.Unlock()
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
		fmt.Printf("Sending msg to peer(%s): ", peer.peerID)
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
	peer.amInterested = true
	return peer.sendMessage(message.NewInterested())
}

func (peer *Peer) sendNotInterested() error {
	peer.amInterested = false
	return peer.sendMessage(message.NewNotInterested())
}

func (peer *Peer) sendChoke() error {
	peer.amChoking = true
	return peer.sendMessage(message.NewChoke())
}

func (peer *Peer) sendUnchoke() error {
	peer.amChoking = false
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

func (peer *Peer) sendRequest(req blockRequest) error {
	return peer.sendMessage(message.NewRequest(&req.info))
}

func (peer *Peer) sendPiece(res blockResponse) error {
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

func (peer *Peer) sendCancel(req blockRequest) error {
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
	return peer.sendMessage(message.NewCancel(&req.info))
}

func (peer *Peer) readMessage() (*message.Message, error) {
	if !peer.Connected {
		return nil, nil
	}
	const MESSAGE_READ_TIMEOUT_MS = 1000
	const MAX_MESSAGE_LENGTH = 17 * 1024 // 17kb for 16 kb max piece length
	msgLenBytes := make([]byte, 4)
	peer.conn.SetReadDeadline(time.Now().Add(MESSAGE_READ_TIMEOUT_MS * time.Millisecond))
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
	msg, err := message.ParseMessage(msgBytes)
	if err != nil {
		return nil, err
	}
	return msg, nil
}
