package peer

import (
	"fmt"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer/insocket"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer/outsocket"
)

type Peer struct {
	IP          string
	Port        int
	ListenPort  int
	Has         []byte
	InfoHash    string
	MyPeerID    string
	PeerID      string
	initialized bool
	outSocket   *outsocket.OutSocket
	inSocket    *insocket.InSocket
}

func New(ip string, port int, listenPort int, infoHash string, peerID string) (*Peer, error) {
	var peer Peer
	peer.IP = ip
	peer.ListenPort = listenPort
	peer.Port = port
	peer.InfoHash = infoHash
	peer.MyPeerID = peerID
	peer.initialized = true
	peer.inSocket = nil
	peer.outSocket = nil
	return &peer, nil
}

func (peer *Peer) Connect() error {
	var err error
	peer.inSocket, err = insocket.New(peer.ListenPort)
	if err != nil {
		fmt.Println("Cannot instantiate in socket")
		return err
	}
	go peer.inSocket.Listen()
	fmt.Printf("my port: %d\n", peer.Port)
	if peer.outSocket, err = outsocket.New(peer.IP, peer.Port, peer.InfoHash); err != nil {
		return err
	}
	if err = peer.outSocket.Connect(); err != nil {
		return err
	}
	return nil
}

func (peer *Peer) Disconnect() []error {
	var errs []error
	err := peer.inSocket.Close()
	if err != nil {
		errs = append(errs, err)
	}
	err = peer.outSocket.Close()

	if err != nil {
		errs = append(errs, err)
	}
	return errs
}
