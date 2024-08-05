// TODO:
// Complete handshake and find session for infohash
// Implement ICE and UPnP

package listenserver

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// use STUN to get public IP and Port
// start listening to port
// reply to handshakes and get peerIDs
// add new peers to peer list

type ListenServer struct {
	internalPort   uint16
	externalPort   uint16
	externalIP     string
	natInitialized bool
	initialized    bool
	running        bool
	stopped        bool
	connChan       chan<- net.Conn
}

func New(internalPort uint16, connChan chan<- net.Conn, externalPort uint16, externalIP string) (*ListenServer, error) {
	lp := ListenServer{
		internalPort: internalPort,
		initialized:  true,
		running:      false,
		stopped:      true,
		connChan:     connChan,
		externalPort: externalPort,
		externalIP:   externalIP,
	}
	return &lp, nil
}

func (ls *ListenServer) listen() {
	// if !ls.natInitialized {
	// 	ls.initializeNAT()
	// }
	fmt.Printf("listenport started on port: %d\n", ls.internalPort)
	listen, err := net.Listen("tcp", fmt.Sprintf("%d", ls.externalPort))
	if err != nil {
		fmt.Println(err)
		return
	}
	defer listen.Close()
	for ls.running {
		conn, err := listen.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}
		go ls.handleConn(conn)
	}
	fmt.Println("Stopped Listening...")
}

func (ls *ListenServer) initializeNAT() error {
	// use stun to get external ip and port
	ls.externalIP = "0.0.0.0"
	ls.externalPort = 6681
	ls.natInitialized = true
	return nil
}

func (ls *ListenServer) handleConn(conn net.Conn) {
	ls.connChan <- conn
}

func (ls *ListenServer) Start(errCh chan<- error) error {
	if !ls.initialized {
		return errors.New("listen port not correctly initialized")
	}
	if ls.running || !ls.stopped {
		return errors.New("listen port already running")
	}
	ls.stopped = false
	ls.running = true
	go ls.listen()
	return nil
}

func (ls *ListenServer) Stop() error {
	ls.running = false
	i := 0
	for !ls.stopped && i < 100 {
		time.Sleep(100 * time.Millisecond)
		i++
	}
	if i == 100 {
		return errors.New("cannot stop listen port")
	}
	return nil
}
