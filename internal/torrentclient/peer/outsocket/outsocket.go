package outsocket

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer/message"
)

type OutSocket struct {
	Port      		int
	IP        		string
	Connected 		bool
	InfoHash  		string
	initialized  	bool
	PeerID 			string
	socket       	net.Conn
}

func New(ip string, port int, infoHash string) (*OutSocket, error) {
	var os OutSocket
	os.IP = ip
	os.Port = port
	os.InfoHash = infoHash
	os.initialized = true
	return &os, nil
}

func (os *OutSocket) Connect() error {
	if !os.initialized {
		return errors.New("out socket not initialized")
	}
	fmt.Printf("out socket started on: %s:%d\n", os.IP, os.Port)
	pstr := "BitTorrent protocol"
	pstrlen := byte(len(pstr))
	reserved := []byte{0,0,0,0,0,0,0,0}

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", os.IP, os.Port))
	if err != nil {
		return err
	}

	os.socket = conn

	handshake := []byte{pstrlen}
	handshake = append(handshake, []byte(pstr)...)
	handshake = append(handshake, reserved...)
	handshake = append(handshake, []byte(os.InfoHash)...)
	handshake = append(handshake, []byte(os.PeerID)...)

	_, err = conn.Write(handshake)
	if err != nil {
		return err
	}
	fmt.Printf("Sent handshake: %s\n", string(handshake))

	time.Sleep(2 * time.Second)

	reply, err := os.ReadReply()
	if err != nil {
		return err
	}
	fmt.Println(string(reply))
	msg := message.Message{Type: message.INTERESTED}
	msgBytes, err := msg.GetBytes()
	if err != nil {
		return err
	}
	_, err = conn.Write(msgBytes)
	if err != nil {
		return err
	}
	time.Sleep(2 * time.Second)

	reply, err = os.ReadReply()
	if err != nil {
		return err
	}
	fmt.Println(string(reply))
	//msg, err := message.ParseMessage(reply)
	//if err != nil {
		//return err
	//}
	//msg.Print()

	return nil
}

func (os *OutSocket) ReadReply() ([]byte, error) {
	buf := make([]byte, 17408)
	n, err := os.socket.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (os *OutSocket) Close() error {
	err := os.socket.Close()
	if err != nil {
		return err
	}
	fmt.Println("OutSocket disconnected")
	return nil
}