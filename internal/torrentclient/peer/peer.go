// TODO:
// DHT
// Add bitfield has changed function

package peer

import (
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
	requestOutQueue		[]BlockRequest
	requestInQueue		[]BlockRequest
	IP					string
	Port				int
	MsgInQueue			[]*message.Message
	requestInChan		chan BlockRequest
	requestOutChan		chan BlockRequest
	requestTimeChan		chan BlockRequest
	replyOutChan		chan BlockReply
	replyOutQueue		[]BlockReply
	replyInChan			chan BlockReply
	Seeding				bool
}

func New(infoHash []byte, numPieces int64, conn net.Conn, peerID string, msgOutChan chan *message.Message, msgInChan chan *message.Message) (*Peer, error) {
	// Received conn from listen port and complete handshake -> add to peer list
	return nil, nil
}

func Connect(infoHash []byte, numPieces int64, ip string, port int, peerID string, myPeerID string, myBitfield *bitfield.BitField, msgOutChan chan *message.Message, msgInChan chan *message.Message) (*Peer, error) {
	peer := Peer{
		InfoHash: infoHash,
		AmInterested: false,
		AmChoking: true,
		PeerChoking: true,
		PeerInterested: false,
		lastMsgSentTime: time.Now(),
		lastMsgReceivedTime: time.Now(),
		PeerID: peerID,
		IP: ip,
		Port: port,
		myBitField: myBitfield,
		MsgInChan: msgInChan,
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

	replyHandshake, err := message.ReadHandshake(conn, 3000 * time.Millisecond)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Handshake received: peerID: %s, infoHash: %x\n", replyHandshake.PeerID, replyHandshake.InfoHash)
	fmt.Printf("Handshake reserved: ")
	for _, b := range replyHandshake.Reserved {
		fmt.Printf("%08b ", b)
	}
	fmt.Print("\n")


	peer.conn = conn
	peer.lastMsgSentTime = time.Now()
	peer.lastMsgReceivedTime = time.Now()
	peer.PeerID = replyHandshake.PeerID
	peer.keepAlive = true
	peer.IsConnected = true

	//bfmsg := message.NewBitfield(peer.myBitField.Bytes)
	//fmt.Printf("bfmsgbytes: %x\n", bfmsg.GetBytes())
	//err = peer.send(bfmsg)
	//if err != nil {
		//return nil, err
	//}

	go peer.handleConn()
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
					peer.requestInChan <- BlockRequest{
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
				for j < len(peer.requestOutQueue) {
					if peer.requestOutQueue[j].Info.PieceIndex == int64(curMsg.Index) && peer.requestOutQueue[j].Info.BeginOffset == int64(curMsg.Begin) {
						//remove request from out queue
						peer.requestOutQueue = append(peer.requestOutQueue[:j], peer.requestOutQueue[j+1:]...)
					}
					j++
				}
				if j == len(peer.requestOutQueue) {
					// Not found so dont process
					continue
				}
				peer.replyInChan <- BlockReply{
					Index: int64(curMsg.Index), 
					Offset: int64(curMsg.Begin),
					Bytes: curMsg.Piece,
				}
			case message.CANCEL:
				j := 0
				for j < len(peer.requestInQueue) {
					// If same request
					if peer.requestInQueue[j].Info.PieceIndex == int64(curMsg.Index) && peer.requestInQueue[j].Info.BeginOffset == int64(curMsg.Begin) {
						// Remove request from in queue
						peer.requestInQueue = append(peer.requestInQueue[:j], peer.requestInQueue[j+1:]...)
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
				peer.replyOutQueue = make([]BlockReply, 0)
				peer.requestInQueue = make([]BlockRequest, 0)
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
			case curReq := <-peer.requestOutChan:
				peer.requestOutQueue = append(peer.requestOutQueue, curReq)
			default:
				bMore = false
			}
		}

		//Send requests
		
		if len(peer.requestOutQueue) > 0 {
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
			for i < len(peer.requestOutQueue) {
				if peer.requestOutQueue[i].Fetching {
					continue
				}
				curReqMsg := message.NewRequest(peer.requestOutQueue[0].Info)
				peer.requestOutQueue[0].Fetching = true
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

		if len(peer.replyOutQueue) > 0{
			if peer.AmChoking {
				continue
			}
			if !peer.PeerInterested {
				peer.replyOutQueue = make([]BlockReply, 0)
				peer.requestInQueue = make([]BlockRequest, 0)
				continue
			}
			for len(peer.replyOutQueue) > 0 {
				j := 0
				for j < len(peer.requestInQueue) {
					//find corresponding request
					if peer.replyOutQueue[0].Index == peer.requestInQueue[j].Info.PieceIndex && peer.replyOutQueue[0].Offset == peer.requestInQueue[j].Info.BeginOffset {
						// Remove request
						peer.requestInQueue = append(peer.requestInQueue[:j], peer.requestInQueue[j+1:]...)
						break
					}
					j++
				}
				// Not found
				if j == len(peer.requestInQueue) {
					//Discard reply
					peer.replyOutQueue = peer.replyOutQueue[1:]
					continue
				}
				err := peer.send(message.NewPiece(uint32(peer.replyOutQueue[0].Index), uint32(peer.replyOutQueue[0].Offset), peer.replyOutQueue[0].Bytes))
				if err != nil {
					fmt.Println("Error sending piece, ", err)
					continue
				}
				peer.replyOutQueue = peer.replyOutQueue[1:]
			}
		}
	}

}


func (peer *Peer) handleConn() {
	for peer.IsConnected {
		if peer.keepAlive {
			select {
			case msg := <-peer.MsgOutChan:
				peer.send(msg)
				if msg.Type == message.PIECE {
					if !peer.Seeding {
						peer.sendChoke()
					}
				}
			default:
				//No message to send
			}
			if len(peer.requestQueue) > 0 {
				if !peer.requestQueue[0].fetching {
					if !peer.AmInterested {
						peer.send(message.NewInterested())
						peer.AmInterested = true
					} else {
						if !peer.PeerChoking {
							peer.send(message.NewRequest(peer.requestQueue[0].info))
							peer.requestQueue[0].fetching = true
						}
					}
				}
			} else {
				if peer.AmInterested {
					peer.SendNotInterested()
				} else {
					if time.Now().After(peer.lastMsgSentTime.Add(15 * time.Second)) {
						err := peer.send(message.NewKeepAlive())
						if err != nil {
							fmt.Printf("Keep alive error: %e", err)
							peer.Close()
							break
						}
					}
					if time.Now().After(peer.lastMsgReceivedTime.Add(30 * time.Second)) {
						fmt.Println("Peer not alive any more... Killing")
						peer.Close()
						break
					}
				}
			}
		}
		if !peer.IsConnected {
			break
		}
		msg, err := message.ReadMessage(peer.conn)
		if err != nil {
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				//No message read / Read timed out
				//fmt.Println("No message read")
				time.Sleep(200 * time.Millisecond)
				continue
			}
			fmt.Println("peer.handleConn error:")
			fmt.Println(err)
			continue
		}
		switch msg.Type {
		case message.CHOKE:
			peer.PeerChoking = true
		case message.UNCHOKE:
			peer.PeerChoking = false
		case message.INTERESTED:
			peer.PeerInterested = true
		case message.NOT_INTERESTED:
			peer.PeerInterested = false
		case message.BITFIELD:
			peer.bitfield = bitfield.LoadBytes(msg.BitField, int64(msg.Length))
		case message.HAVE:
			if !peer.HasBitField() {
				peer.bitfield = bitfield.New(peer.myBitField.Len())
			}
			peer.bitfield.SetBit(int64(msg.Index))
		case message.PIECE:
			//Send piece to session
			peer.MsgInChan<-msg
			peer.requestQueue = peer.requestQueue[1:]
			if peer.PeerInterested {
				err = peer.sendUnchoke()
				if err != nil {
					fmt.Println("Error sending unchoke, ", err)
				}
			}
		case message.REQUEST:
			peer.MsgInChan<-msg
		}
		peer.lastMsgReceivedTime = time.Now()
		if PEER_DEBUG {
			fmt.Printf("Peer (%s) Got message: ", peer.PeerID)
			msg.Print()
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

func (peer *Peer) DownloadBlock(bi *bundle.BlockInfo) error {
	if !peer.HasPiece(bi.PieceIndex) {
		return errors.New("peer doesn't have piece")
	}
	peer.requestQueue = append(peer.requestQueue, BlockRequest{info: bi, fetching: false})
	if PEER_DEBUG {
		fmt.Printf("Added req to queue on peer(%s)\n", string(peer.PeerID))
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