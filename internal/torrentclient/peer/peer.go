// Keepalive not working
package peer

import (
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

type BlockReq struct {
	info *bundle.BlockInfo
	fetching bool
}

type Peer struct {
	AmInterested 			bool
	AmChoked 				bool
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
	requestQueue		[]BlockReq
	IP					string
	Port				int
}

func New(infoHash []byte, numPieces int64, conn net.Conn, peerID string) (*Peer, error) {
	peer := Peer{
		InfoHash: infoHash,
		AmInterested: false, 
		AmChoked: true, 
		PeerChoking: true, 
		PeerInterested: false, 
		bitfield: bitfield.New(numPieces),
		conn: conn,
		isConnected: true,
		keepAlive: true,
		PeerID: peerID,
	}
	go peer.handleConn()
	return &peer, nil
}

func Connect(infoHash []byte, numPieces int64, ip string, port int, peerID string, myPeerID string) (*Peer, error) {
	peer := Peer{
		InfoHash: infoHash,
		AmInterested: false,
		AmChoked: true,
		PeerChoking: true,
		PeerInterested: false,
		bitfield: bitfield.New(numPieces),
		lastMsgSentTime: time.Now(),
		lastMsgReceivedTime: time.Now(),
		PeerID: peerID,
		IP: ip,
		Port: port,
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
			if len(peer.requestQueue) > 0 && !peer.requestQueue[0].fetching {
				if peer.PeerChoking {
					peer.send(message.NewInterested())
				} else {
					peer.send(message.NewRequest(peer.requestQueue[0].info))
					peer.requestQueue[0].fetching = true
				}
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
		if !peer.isConnected {
			break
		}
		msg, err := message.ReadMessage(peer.conn)
		if err != nil {
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				//No message read / Read timed out
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
			peer.bitfield = bitfield.LoadBytes(msg.BitField, int64(msg.Length))
		case message.PIECE:
			//Send piece to session
			fmt.Printf("GOT BLOCK!!!! Index: %d, Offset: %d, Length: %d\n", msg.Index, msg.Begin, msg.Length)
			peer.requestQueue = peer.requestQueue[1:]
		}
		peer.lastMsgReceivedTime = time.Now()
		fmt.Printf("Got message: ")
		msg.Print()
	}
}

//Message interface

func (peer *Peer) send(msg *message.Message) error {
	_, err := peer.conn.Write(msg.GetBytes())
	if err != nil {
		return err
	}
	peer.lastMsgSentTime = time.Now()
	if PEER_DEBUG {
		fmt.Printf("Sent message to peer (%s): ", peer.PeerID)
		msg.Print()
	}
	return nil
}

func (peer *Peer) DownloadBlock(bi *bundle.BlockInfo) error {
	if !peer.HasPiece(bi.PieceIndex) {
		return errors.New("peer doesn't have piece")
	}
	peer.requestQueue = append(peer.requestQueue, BlockReq{info: bi, fetching: false})
	if PEER_DEBUG {
		fmt.Printf("Added req to queue on peer(%s)\n", string(peer.PeerID))
	}
	return nil
}

func (peer *Peer) HasPiece(pieceIndex int64) bool {
	if peer.bitfield.Full {
		return true
	}
	return peer.bitfield.GetBit(pieceIndex)
}

func (peer *Peer) NumPieces() int64 {
	return peer.bitfield.NumSet
}