// TODO:
// Complete handshake and find session for infohash
// Implement ICE and UPnP

package listenserver

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/logger"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer/message"
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
	PeerChan       chan *IncomingPeer

	logger *logger.Logger
}

type IncomingPeer struct {
	PeerID      []byte
	InfoHash    []byte
	ConnectTime time.Time
	Conn        net.Conn
}

func New(internalPort uint16, externalPort uint16, externalIP string) (*ListenServer, error) {
	lp := ListenServer{
		internalPort: internalPort,
		initialized:  true,
		running:      false,
		stopped:      true,
		externalPort: externalPort,
		externalIP:   externalIP,
		PeerChan:     make(chan *IncomingPeer, 20),
		logger:       logger.New(logger.DEBUG, "ListenServer"),
	}
	return &lp, nil
}

func (ls *ListenServer) listen() {
	// if !ls.natInitialized {
	// 	ls.initializeNAT()
	// }
	fmt.Printf("listenport started on port: %d\n", ls.internalPort)
	listen, err := net.Listen("tcp", fmt.Sprintf("%d", ls.internalPort))
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
	const HANDSHAKE_TIMEOUT_MS = 5000
	conn.SetReadDeadline(time.Now().Add(HANDSHAKE_TIMEOUT_MS * time.Millisecond))
	handshakeBytes := make([]byte, 68)
	n, err := conn.Read(handshakeBytes)
	if err != nil {
		ls.logger.Error(fmt.Sprintf("error reading handshake from incoming peer: %v", err))
		return
	}
	if n != 68 {
		ls.logger.Error("error reading handshake from incoming peer: invalid handshake length")
	}
	ls.logger.Debug(fmt.Sprintf("Read handshake bytes from peer: %x\n", handshakeBytes))
	handshakeMsg, err := message.ParseHandshake(handshakeBytes)
	if err != nil {
		ls.logger.Error(fmt.Sprintf("error reading handshake from incoming peer: %v", err))
		return
	}

	ls.logger.Debug(fmt.Sprintf("Handshake successfully read from peer(%s)...\n", conn.RemoteAddr().String()))
	newPeer := IncomingPeer{
		PeerID:      handshakeMsg.PeerID,
		InfoHash:    handshakeMsg.InfoHash,
		ConnectTime: time.Now(),
		Conn:        conn,
	}
	ls.PeerChan <- &newPeer
}

func (ls *ListenServer) Start() error {
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
