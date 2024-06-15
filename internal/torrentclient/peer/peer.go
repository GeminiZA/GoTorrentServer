// Keepalive not working
package peer

import (
	"fmt"
	"net"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer/message"
)

const PEER_DEBUG bool = true
const HANDSHAKE_REPLY_TIMEOUT_MS = 3000

type Peer struct {
	Interested 			bool
	Choked 				bool
	PeerChoking 		bool
	PeerInterested  	bool
	InfoHash    		[]byte
	PeerID      		string
	conn				net.Conn
	isConnected 		bool
	bitfield 			*bitfield.BitField
	keepAlive 			bool
	lastMsgSentTime		time.Time
	lastMsgReceivedTime time.Time
	msgChan				chan<-*message.Message
}

func New(infoHash []byte, numPieces int, conn net.Conn, msgChan chan<-*message.Message) (*Peer, error) {
	peer := Peer{
		InfoHash: infoHash,
		Interested: false, 
		Choked: true, 
		PeerChoking: true, 
		PeerInterested: false, 
		bitfield: bitfield.New(numPieces),
		conn: conn,
		isConnected: true,
		keepAlive: true,
	}
	go peer.handleConn()
	return &peer, nil
}

func Connect(infoHash []byte, numPieces int, ip string, port int, myPeerID string, msgChan chan<-*message.Message) (*Peer, error) {
	peer := Peer{
		InfoHash: infoHash,
		Interested: false,
		Choked: true,
		PeerChoking: true,
		PeerInterested: false,
		bitfield: bitfield.New(numPieces),
		lastMsgSentTime: time.Now(),
		lastMsgReceivedTime: time.Now(),
		msgChan: msgChan,
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
	fmt.Printf("Handshake received: peerID: %s, infoHash: %s\n", replyHandshake.PeerID, string(replyHandshake.InfoHash))
	fmt.Printf("Handshake reserved: ")
	for _, b := range replyHandshake.Reserved {
		fmt.Printf("%08b ", b)
	}
	fmt.Print("\n")

	peer.lastMsgSentTime = time.Now()
	peer.lastMsgReceivedTime = time.Now()
	peer.conn = conn
	peer.PeerID = replyHandshake.PeerID
	peer.keepAlive = true
	peer.isConnected = true
	go peer.handleConn()
	return &peer, nil
}

func (peer *Peer) Close() {
	peer.keepAlive = false
	peer.isConnected = false
	peer.conn.Close()
}

func (peer *Peer) IsConnected() bool {
	return peer.isConnected
}

func (peer *Peer) handleConn() {
	for peer.isConnected {
		if peer.keepAlive {
			if time.Now().After(peer.lastMsgSentTime.Add(15 * time.Second)) {
				err := peer.SendKeepAlive()
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
		msg, err := message.ReadMessage(peer.conn)
		if err != nil {
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				//if PEER_DEBUG {
					//fmt.Println("no message read")
				//}
				time.Sleep(time.Second)
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
			peer.bitfield = bitfield.LoadBytes(msg.BitField)
		case message.KEEP_ALIVE:
			//Todo
		default:
			peer.msgChan<-msg
		}
		peer.lastMsgReceivedTime = time.Now()
		fmt.Printf("Got message: ")
		msg.Print()
	}
}

//Message interface

func (peer *Peer) SendInterested() error {
	msg := message.NewInterested()
	_, err := peer.conn.Write(msg.GetBytes())
	if err != nil {
		return err
	}
	peer.lastMsgSentTime = time.Now()
	peer.Interested = true
	if PEER_DEBUG {
		fmt.Printf("Sent Interested to: %s\n", peer.PeerID)
	}
	return nil
}

func (peer *Peer) SendNotInterested() error {
	msg := message.NewNotInterested()
	_, err := peer.conn.Write(msg.GetBytes())
	if err != nil {
		return err
	}
	peer.lastMsgSentTime = time.Now()
	peer.Interested = false
	if PEER_DEBUG {
		fmt.Printf("Sent Interested to: %s\n", peer.PeerID)
	}
	return nil
}

func (peer *Peer) SendUnchoke() error {
	msg := message.NewUnchoke()
	_, err := peer.conn.Write(msg.GetBytes())
	if err != nil {
		return err
	}
	peer.lastMsgSentTime = time.Now()
	peer.Choked = false
	if PEER_DEBUG {
		fmt.Printf("Sent Unchoke to: %s\n", peer.PeerID)
	}
	return nil
}

func (peer *Peer) SendChoke() error {
	msg := message.NewChoke()
	_, err := peer.conn.Write(msg.GetBytes())
	if err != nil {
		return err
	}
	peer.lastMsgSentTime = time.Now()
	peer.Choked = true
	if PEER_DEBUG {
		fmt.Printf("Sent Choke to: %s\n", peer.PeerID)
	}
	return nil
}

func (peer *Peer) SendRequestBlock(pieceIndex int64, beginOffset int64, length int64) error {
	msg := message.NewRequest(pieceIndex, beginOffset, length)
	msgBytes := msg.GetBytes()
	_, err := peer.conn.Write(msgBytes)
	if err != nil {
		return err
	}
	peer.lastMsgSentTime = time.Now()
	if PEER_DEBUG {
		fmt.Printf("Sent Request to: %s : PieceIndex: %d, Offset: %d, Length: %d\n", peer.PeerID, pieceIndex, beginOffset, length)
		fmt.Printf("Request hex: %x Length: %d\n", msgBytes, len(msgBytes))
	}
	return nil
}

func (peer *Peer) SendBitField(bf *bitfield.BitField) error {
	msg := message.NewBitfield(bf.Bytes)
	msgBytes := msg.GetBytes()
	_, err := peer.conn.Write(msgBytes)
	if err != nil {
		return err
	}
	peer.lastMsgSentTime = time.Now()
	if PEER_DEBUG {
		fmt.Print("Sent bitfield...\n")
	}
	return nil
}

func (peer *Peer) SendKeepAlive() error {
	msg := message.NewKeepAlive()
	msgBytes := msg.GetBytes()
	_, err := peer.conn.Write(msgBytes)
	if err != nil {
		return err
	}
	peer.lastMsgSentTime = time.Now()
	if PEER_DEBUG {
		fmt.Print("Sent keep alive...\n")
	}
	return nil
}