package peer

import (
	"encoding/binary"
	"fmt"
	"io"
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
}

func New(infoHash []byte, numPieces int, conn net.Conn) (*Peer, error) {
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

func Connect(infoHash []byte, numPieces int, ip string, port int, myPeerID string) (*Peer, error) {
	peer := Peer{
		InfoHash: infoHash,
		Interested: false,
		Choked: true,
		PeerChoking: true,
		PeerInterested: false,
		bitfield: bitfield.New(numPieces),
		lastMsgTime: time.Now(),
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

	handshakeReplyBytes := []byte{}
	//Read pstrlen
	pstrlenBytes := make([]byte, 1)
	deadline := time.Now().Add(timeoutDuration)
	err = conn.SetReadDeadline(deadline)
	if err != nil {
		return nil, err
	}
	_, err = conn.Read(pstrlenBytes)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			fmt.Println("handshake timed out...")
			conn.Close()
			return nil, err
		} else {
			conn.Close()
			return nil, err
		}
	}
	pstrlen := pstrlenBytes[0]
	fmt.Printf("Read pstrlen: %x\n", pstrlen)
	handshakeReplyBytes = append(handshakeReplyBytes, pstrlenBytes...)
	rest := make([]byte, 48+pstrlen)
	//Read pstr
	_, err = conn.Read(rest)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			fmt.Println("handshake timed out...")
			conn.Close()
			return nil, err
		} else {
			conn.Close()
			return nil, err
		}
	}
	handshakeReplyBytes = append(handshakeReplyBytes, rest...)

	if PEER_DEBUG {
		fmt.Printf("Got handshake reply (len: %d): %s\n", len(handshakeReplyBytes), string(handshakeReplyBytes))
	}
	replyMsg, err := message.ParseHandshake(handshakeReplyBytes)
	if err != nil {
		conn.Close()
		return nil, err
	}

	fmt.Printf("Handshake reserved: ")
	for _, b := range replyMsg.Reserved {
		fmt.Printf("%08b ", b)
	}
	fmt.Print("\n")

	peer.conn = conn
	peer.PeerID = replyMsg.PeerID
	peer.keepAlive = true
	peer.isConnected = true
	//go peer.keepPeerAlive()
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

func (peer *Peer) HandleHandshake() error {
	
}

func (peer *Peer) handleConn() {
	for peer.isConnected {
		fmt.Println("Waiting for message...")
		msglenBytes := make([]byte, 4)
		_, err := peer.conn.Read(msglenBytes)
		if err != nil {
			fmt.Println("peer.handleConn error:")
			fmt.Println(err)
			if err == io.EOF {
				fmt.Println("peer closed the connection")
				return
			}
			return
		}
		msglen := binary.BigEndian.Uint32(msglenBytes)
		fmt.Printf("Reading message length:  %d\n", msglen)
		buf := make([]byte, msglen)
		n, err := peer.conn.Read(buf)
		if err != nil {
			fmt.Println("peer.handleConn error:")
			fmt.Println(err)
			if err == io.EOF {
				fmt.Println("peer closed the connection")
				return
			}
			return
		}
		fmt.Printf("Got message (len: %d): %x\n", n,  buf[:n])
		msg, err := message.ParseMessage(buf[:n])
		if err != nil {
			fmt.Println("peer.handleConn error:")
			fmt.Println(err)
			continue
		}
		msg.Print()
	}
}

func (peer *Peer) keepPeerAlive() {
	for peer.keepAlive {
		if !peer.isConnected {
			return
		}
		if time.Now().Add(time.Second * 15).After(peer.lastMsgTime) {
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