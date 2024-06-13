package peer

import (
	"fmt"
	"net"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer/message"
)

const PEER_DEBUG bool = true
const HANDSHAKE_REPLY_TIMEOUT_MS = 1000

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
	}
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
	}
	if PEER_DEBUG {
		fmt.Printf("Trying to connect to peer on: %s:%d\n", ip, port)
	}

	handshake := message.NewHandshake(infoHash, myPeerID)

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return nil, err
	}
	
	_, err = conn.Write(handshake.GetBytes())
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 17408) // 17kb

	timeoutDuration := HANDSHAKE_REPLY_TIMEOUT_MS * time.Millisecond
	deadline := time.Now().Add(timeoutDuration)
	err = conn.SetReadDeadline(deadline)
	if err != nil {
		return nil, err
	}
	n, err := conn.Read(buf)

	return nil, nil
}

func (peer *)