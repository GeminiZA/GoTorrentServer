// TODO:
// DHT
// Add bitfield has changed function
// Check for race conditions in handling functions

package peer

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer/message"
)

const PEER_DEBUG bool = true
const HANDSHAKE_REPLY_TIMEOUT_MS = 3000

type BlockRequest struct {
	Info *bundle.BlockInfo
	Fetching bool
	TimeRequested time.Time
	TimesRequested byte
}

type BlockReply struct {
	Index int64
	Offset int64
	Bytes []byte
}

type Peer struct {
	AmInterested 			bool
	AmChoking 				bool
	PeerChoking 		bool
	PeerInterested  	bool
	InfoHash    		[]byte
	PeerID      		string
	conn				net.Conn
	IsConnected 		bool
	bitfield 			*bitfield.BitField
	myBitField			*bitfield.BitField
	keepAlive 			bool
	lastMsgSentTime		time.Time
	lastMsgReceivedTime time.Time
	IP					string
	Port				int
	MsgInQueue			[]*message.Message
	RequestInChan		chan BlockRequest
	RequestInQueue		[]BlockRequest
	ReplyInChan			chan BlockReply
	RequestOutChan		chan BlockRequest
	RequestOutQueue		[]BlockRequest
	RequestTimedChan	chan BlockRequest
	ReplyOutChan		chan BlockReply
	ReplyOutQueue		[]BlockReply
	Seeding				bool
}

//To peer channels
//Send BlockRequests
//Send BlockReplies
//Get BlockRequests
//Get BlockReplies


func New(infoHash []byte, numPieces int64, conn net.Conn, peerID string, msgOutChan chan *message.Message, msgInChan chan *message.Message) (*Peer, error) {
	// Received conn from listen port and complete handshake -> add to peer list
	return nil, nil
}

func Connect(infoHash []byte, numPieces int64, ip string, port int, peerID string, myPeerID string, myBitfield *bitfield.BitField) (*Peer, error) {
	peer := Peer{
		AmInterested: false,
		AmChoking: true,
		PeerChoking: true,
		PeerInterested: false,
		InfoHash: infoHash,
		PeerID: peerID,
		IsConnected: false,
		myBitField: myBitfield,
		IP: ip,
		Port: port,
		MsgInQueue: make([]*message.Message, 0),
		RequestInChan: make(chan BlockRequest, 100),
		RequestOutChan: make(chan BlockRequest, 100),
		RequestOutQueue: make([]BlockRequest, 0),
		RequestInQueue: make([]BlockRequest, 0),
		RequestTimedChan: make(chan BlockRequest, 100),
		ReplyOutChan: make(chan BlockReply, 100),
		ReplyOutQueue: make([]BlockReply, 0),
		ReplyInChan: make(chan BlockReply, 100),
		Seeding: false,
	}
	if PEER_DEBUG {
		fmt.Printf("Trying to connect to peer on: %s:%d\n", ip, port)
	}

	handshake := message.NewHandshake(infoHash, myPeerID)

	if PEER_DEBUG {
		fmt.Printf("handshake bytes: %s\n", handshake.GetBytes())
	}

	timeoutDuration := HANDSHAKE_REPLY_TIMEOUT_MS * time.Millisecond
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), timeoutDuration)
	if err != nil {
		return nil, err
	}
	if PEER_DEBUG {
		fmt.Println("conn established...")
	}
	
	_, err = conn.Write(handshake.GetBytes())
	if err != nil {
		return nil, err
	}
	if PEER_DEBUG {
		fmt.Println("Wrote handshake...")
	}

	handshakeReplyBytes := make([]byte, 68)
	conn.SetReadDeadline(time.Now().Add(HANDSHAKE_REPLY_TIMEOUT_MS * time.Millisecond))
	n, err := conn.Read(handshakeReplyBytes)
	if err != nil {
		return nil, err
	}
	if n != 68 {
		return nil, errors.New("didnt read 68 bytes in handshake reply")
	}

	replyHandshake, err := message.ParseHandshake(handshakeReplyBytes)
	if err != nil {
		return nil, err
	}

	if replyHandshake.PeerID != peer.PeerID {
		return nil, errors.New("peerID mismatch")
	}
	if !bytes.Equal(replyHandshake.InfoHash, peer.InfoHash) {
		return nil, errors.New("infoHash mismatch")
	}
	if PEER_DEBUG {
		fmt.Printf("Handshake received: peerID: %s, infoHash: %x\n", replyHandshake.PeerID, replyHandshake.InfoHash)
		fmt.Printf("Handshake reserved: ")
		for _, b := range replyHandshake.Reserved {
			fmt.Printf("%08b ", b)
		}
		fmt.Print("\n")
	}

	peer.conn = conn
	peer.lastMsgSentTime = time.Now()
	peer.lastMsgReceivedTime = time.Now()
	peer.keepAlive = true
	peer.IsConnected = true

	bfmsg := message.NewBitfield(peer.myBitField.Bytes)
	if PEER_DEBUG {
		fmt.Printf("bfmsgbytes: %x\n", bfmsg.GetBytes())
	}
	err = peer.send(bfmsg)
	if err != nil {
		return nil, err
	}

	go peer.handleInbound()
	go peer.handleOutbound()

	return &peer, nil
}

func (peer *Peer) Close() {
	peer.keepAlive = false
	peer.IsConnected = false
	peer.conn.Close()
}

func (peer *Peer) hasPiecesIDont() bool {
	// (NOT mybitfield AND their bitfield).NumSet == 0
	return peer.myBitField.NotAANDB(peer.bitfield).NumSet == 0
}

func (peer *Peer) readMessage() error {
	const READ_TIMEOUT_MS = 1000
	msgLenBytes := make([]byte, 4)

	peer.conn.SetReadDeadline(time.Now().Add(READ_TIMEOUT_MS * time.Millisecond))

	n, err := peer.conn.Read(msgLenBytes)
	if err != nil {
		return err
	}
	if n != 4 {
		return errors.New("message length not 4 bytes")
	}
	bytesToRead := int(binary.BigEndian.Uint32(msgLenBytes))
	if bytesToRead == 0 {
		peer.MsgInQueue = append(peer.MsgInQueue, message.NewKeepAlive())
		return nil
	}
	bytesRead := 0
	msgBytes := make([]byte, 0)
	buf := make([]byte, bytesRead)
	for bytesRead < bytesToRead {
		n, err := peer.conn.Read(buf)
		if err != nil {
			return err
		}
		bytesRead += n
		msgBytes = append(msgBytes, buf[:n]...)
	}
	msg, err := message.ParseMessage(msgBytes)
	if err != nil {
		return err
	}
	peer.MsgInQueue = append(peer.MsgInQueue, msg)
	return nil
}


func (peer *Peer) handleInbound() {
	for peer.IsConnected {
		err := peer.readMessage()
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			continue
		}
		for len(peer.MsgInQueue) > 0 {
			curMsg := peer.MsgInQueue[0]
			switch curMsg.Type {
			case message.BITFIELD:
				peer.bitfield = bitfield.LoadBytes(curMsg.BitField, peer.NumPieces())
			case message.REQUEST:
				if !peer.AmChoking {
					peer.RequestInChan <- BlockRequest{
						Info: &bundle.BlockInfo{
							PieceIndex: int64(curMsg.Index), 
							BeginOffset: int64(curMsg.Begin), 
							Length: int64(curMsg.Length)}, 
						Fetching: false,
						TimeRequested: time.Now(),
						TimesRequested: 0,
					}
				}
			case message.PIECE:
				j := 0
				for j < len(peer.RequestOutQueue) {
					if peer.RequestOutQueue[j].Info.PieceIndex == int64(curMsg.Index) && peer.RequestOutQueue[j].Info.BeginOffset == int64(curMsg.Begin) {
						//remove request from out queue
						peer.RequestOutQueue = append(peer.RequestOutQueue[:j], peer.RequestOutQueue[j+1:]...)
					}
					j++
				}
				if j == len(peer.RequestOutQueue) {
					// Not found so dont process
					continue
				}
				peer.ReplyInChan <- BlockReply{
					Index: int64(curMsg.Index), 
					Offset: int64(curMsg.Begin),
					Bytes: curMsg.Piece,
				}
			case message.CANCEL:
				j := 0
				for j < len(peer.RequestInQueue) {
					// If same request
					if peer.RequestInQueue[j].Info.PieceIndex == int64(curMsg.Index) && peer.RequestInQueue[j].Info.BeginOffset == int64(curMsg.Begin) {
						// Remove request from in queue
						peer.RequestInQueue = append(peer.RequestInQueue[:j], peer.RequestInQueue[j+1:]...)
						break
					}
					j++
				}
			case message.CHOKE:
				peer.PeerChoking = true
			case message.UNCHOKE:
				peer.PeerChoking = false
			case message.INTERESTED:
				peer.PeerInterested = true
			case message.NOT_INTERESTED:
				peer.ReplyOutQueue = make([]BlockReply, 0)
				peer.RequestInQueue = make([]BlockRequest, 0)
				peer.PeerInterested = false
			case message.HAVE:
				peer.bitfield.SetBit(int64(curMsg.Index))
			case message.KEEP_ALIVE:
				// Do nothing
			}
			peer.lastMsgReceivedTime = time.Now()
			peer.MsgInQueue = peer.MsgInQueue[1:]
		}
	}
}

func (peer *Peer) handleOutbound() {
	for peer.IsConnected {
		// Fetch messages from session

		bMore := true
		for bMore {
			select {
			case curReq := <-peer.RequestOutChan:
				peer.RequestOutQueue = append(peer.RequestOutQueue, curReq)
			default:
				bMore = false
			}
		}

		//Send requests
		
		if len(peer.RequestOutQueue) > 0 {
			if !peer.AmInterested {
				err := peer.sendInterested()
				if err != nil {
					fmt.Println("error sending intereseted: ", err)
				}
				continue
			}
			if peer.PeerChoking {
				fmt.Println("peer still choking... waiting 100ms...")
				time.Sleep(100 * time.Millisecond)
				continue
			}
			i := 0
			for i < len(peer.RequestOutQueue) {
				if peer.RequestOutQueue[i].Fetching {
					continue
				}
				curReqMsg := message.NewRequest(peer.RequestOutQueue[0].Info)
				peer.RequestOutQueue[0].Fetching = true
				err := peer.send(curReqMsg)
				if err != nil {
					fmt.Println("error sending request: ", err)
					continue
				}
				i++
			}
		} else {
			if !peer.hasPiecesIDont() {
				err := peer.SendNotInterested()
				if err != nil {
					fmt.Println("error sending not intereseted: ", err)
				}
			}
		}

		//Send pieces

		if len(peer.ReplyOutQueue) > 0{
			if peer.AmChoking {
				continue
			}
			if !peer.PeerInterested {
				peer.ReplyOutQueue = make([]BlockReply, 0)
				peer.RequestInQueue = make([]BlockRequest, 0)
				continue
			}
			for len(peer.ReplyOutQueue) > 0 {
				j := 0
				for j < len(peer.RequestInQueue) {
					//find corresponding request
					if peer.ReplyOutQueue[0].Index == peer.RequestInQueue[j].Info.PieceIndex && peer.ReplyOutQueue[0].Offset == peer.RequestInQueue[j].Info.BeginOffset {
						// Remove request
						peer.RequestInQueue = append(peer.RequestInQueue[:j], peer.RequestInQueue[j+1:]...)
						break
					}
					j++
				}
				// Not found
				if j == len(peer.RequestInQueue) {
					//Discard reply
					peer.ReplyOutQueue = peer.ReplyOutQueue[1:]
					continue
				}
				err := peer.send(message.NewPiece(uint32(peer.ReplyOutQueue[0].Index), uint32(peer.ReplyOutQueue[0].Offset), peer.ReplyOutQueue[0].Bytes))
				if err != nil {
					fmt.Println("Error sending piece, ", err)
					continue
				}
				peer.ReplyOutQueue = peer.ReplyOutQueue[1:]
			}
		}
	}

}

//Message interface

func (peer *Peer) send(msg *message.Message) error {
	//if PEER_DEBUG {
		//fmt.Print("Sending message: ")
		//msg.Print()
	//}
	_, err := peer.conn.Write(msg.GetBytes())
	if err != nil {
		return err
	}
	peer.lastMsgSentTime = time.Now()
	if PEER_DEBUG {
		fmt.Printf("Peer (%s) Sent message: ", peer.PeerID)
		msg.Print()
	}
	return nil
}

func (peer *Peer) sendInterested() error {
	msg := message.NewInterested()
	err := peer.send(msg)
	if err != nil {
		return err
	}
	peer.AmInterested = true
	return nil
}

func (peer *Peer) SendNotInterested() error {
	msg := message.NewNotInterested()
	err := peer.send(msg)
	if err != nil {
		return err
	}
	peer.AmInterested = false
	return nil
}

func (peer *Peer) sendChoke() error {
	msg := message.NewChoke()
	err := peer.send(msg)
	if err != nil {
		return err
	}
	peer.AmChoking = true
	return nil
}

func (peer *Peer) sendUnchoke() error {
	msg := message.NewUnchoke()
	err := peer.send(msg)
	if err != nil {
		return err
	}
	peer.AmChoking = true
	return nil
}

func (peer *Peer) HasPiece(pieceIndex int64) bool {
	if peer.HasBitField() {
		if peer.bitfield.Full {
			return true
		}
		return peer.bitfield.GetBit(pieceIndex)
	}
	return false
}

func (peer *Peer) HasBitField() bool {
	return peer.bitfield != nil
}

func (peer *Peer) NumPieces() int64 {
	if peer.HasBitField() {
		return peer.bitfield.NumSet
	}
	return 0
}