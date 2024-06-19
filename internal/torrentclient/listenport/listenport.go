// TODO:
// Complete handshake and find session for infohash
// Implement ICE and UPnP

package listenport

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
)

// use STUN to get public IP and Port
// start listening to port
// reply to handshakes and get peerIDs
// add new peers to peer list

type ListenPort struct {
	internalPort int
	externalPort int
	externalIP string
	natInitialized bool
	initialized bool
	running bool
	stopped bool
	newPeerChan chan<-*peer.Peer
}

func New(internalPort int, peerChannel chan<-*peer.Peer) (*ListenPort, error) {
	lp := ListenPort{
		internalPort: internalPort, 
		initialized: true, 
		running: false, 
		stopped: true,
	}
	return &lp, nil
}

func (lp *ListenPort) listen(peerCh chan<-*peer.Peer, errCh chan<-error) {
	if !lp.natInitialized {
		lp.initializeNAT()
	}
	fmt.Printf("listenport started on port: %d\n", lp.internalPort)
	listen, err := net.Listen("tcp", fmt.Sprintf("%d", lp.externalPort))
	if err != nil {
		errCh <- err
		return
	}
	defer listen.Close()
	for lp.running {
		conn, err := listen.Accept()
		if err != nil {
			errCh<-err
			continue
		}
		go handleConn(conn, peerCh, errCh)
	}
	fmt.Println("Stopped Listening...")
}

func (lp *ListenPort) initializeNAT() error {
	//use stun to get external ip and port
	lp.externalIP = "0.0.0.0"
	lp.externalPort = 6681
	lp.natInitialized = true
	return nil
}

func handleConn(conn net.Conn, peerCh chan<-*peer.Peer, errCh chan<-error) {
	
}

func (lp *ListenPort) Start(errCh chan<-error) error {
	if !lp.initialized {
		return errors.New("listen port not correctly initialized")
	}
	if lp.running || !lp.stopped {
		return errors.New("listen port already running")
	}
	lp.stopped = false
	lp.running = true
	go lp.listen(lp.newPeerChan, errCh)
	return nil
}

func (lp *ListenPort) Stop() error {
	lp.running = false
	i := 0
	for !lp.stopped && i < 100 {
		time.Sleep(100 * time.Millisecond)
		i++
	}
	if i == 100 {
		return errors.New("cannot stop listen port")
	}
	return nil
}