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
	Interested 		bool
	Choked 			bool
	PeerChoking 	bool
	PeerInterested  bool
	InfoHash    	[]byte
	PeerID      	string
	conn			net.Conn
	isConnected 	bool
	bitfield 		*bitfield.BitField
	keepAlive 		bool
	lastMsgTime		time.Time
	msgChan			chan<-*message.Message
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
	go peer.keepPeerAlive()
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
		lastMsgTime: time.Now(),
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
	peer.lastMsgTime = time.Now()
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

	peer.conn = conn
	peer.PeerID = replyHandshake.PeerID
	peer.keepAlive = true
	peer.isConnected = true
	go peer.keepPeerAlive()
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
		fmt.Println("Waiting for message...")
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
			time.Sleep(2 * time.Second)
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
		}
		peer.msgChan<-msg
		fmt.Printf("Got message :")
		msg.Print()
	}
}

func (peer *Peer) keepPeerAlive() {
	for peer.keepAlive {
		if !peer.isConnected {
			return
		}
		if peer.lastMsgTime.After(time.Now().Add(time.Second * 15)) {
			_, err := peer.conn.Write(message.NewKeepAlive().GetBytes())
			if err != nil {
				fmt.Println(err)
				fmt.Printf("Peer (%s) disconnected\n", peer.PeerID)
			}
			fmt.Printf("Wrote keep alive to: %s\n", peer.PeerID)
		}
		time.Sleep(time.Second) //every second
	}
}

//Message interface

func (peer *Peer) SendInterested() error {
	msg := message.NewInterested()
	_, err := peer.conn.Write(msg.GetBytes())
	if err != nil {
		return err
	}
	peer.lastMsgTime = time.Now()
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
	peer.lastMsgTime = time.Now()
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
	peer.lastMsgTime = time.Now()
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
	peer.lastMsgTime = time.Now()
	peer.Choked = true
	if PEER_DEBUG {
		fmt.Printf("Sent Choke to: %s\n", peer.PeerID)
	}
	return nil
}

func (peer *Peer) SendRequestBlock(pieceIndex int64, beginOffset int64, length int64) error {
	msg := message.NewRequest(pieceIndex, beginOffset, length)
	_, err := peer.conn.Write(msg.GetBytes())
	if err != nil {
		return err
	}
	peer.lastMsgTime = time.Now()
	if PEER_DEBUG {
		fmt.Printf("Sent Request to: %s\n", peer.PeerID)
	}
	return nil
}