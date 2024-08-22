package main

import (
	"encoding/hex"
	"fmt"
	"net"
)

const (
	peerID      = "ffffffffffffffffffff"
	infohashHex = "d984f67af9917b214cd8b6048ab5624c7df6a07a"
	remoteIP    = "127.0.0.1"
	remotePort  = 6881
)

func SendHandshake(conn net.Conn) error {
	pstr := []byte("BitTorrent protocol")
	pstrlen := byte(19)
	reserved := make([]byte, 8)
	infoHash, err := hex.DecodeString(infohashHex)
	if err != nil {
		return err
	}
	peerID := []byte(peerID)
	hsBytes := []byte{pstrlen}
	hsBytes = append(hsBytes, pstr...)
	hsBytes = append(hsBytes, reserved...)
	hsBytes = append(hsBytes, infoHash...)
	hsBytes = append(hsBytes, peerID...)

	if err != nil {
		return err
	}

	_, err = conn.Write(hsBytes)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", remoteIP, remotePort))
	if err != nil {
		panic(err)
	}
	err = SendHandshake(conn)
	if err != nil {
		panic(err)
	}
	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Got response: %s\n", string(buf[:n]))
	buf = make([]byte, 128)
	n, err = conn.Read(buf)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Got response: %s\n", string(buf[:n]))
}
