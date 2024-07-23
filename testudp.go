package main

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"time"
)

func main() {
	address := "udp://tracker.opentrackr.org:1337/announce"
	url, err := url.Parse(address)
	if err != nil {
		panic(err)
	}
	addr, err := net.ResolveUDPAddr("udp4", url.Host)
	if err != nil {
		panic(err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Conn created to %v\n", url.Host)
	// {constant (8), action (4), transaction id (4)}
	connectRequestBytes := make([]byte, 16)
	binary.BigEndian.PutUint64(connectRequestBytes[0:], 0x41727101980)
	binary.BigEndian.PutUint32(connectRequestBytes[8:], 0)
	transactionID := uint32(rand.Int())
	binary.BigEndian.PutUint32(connectRequestBytes[12:], transactionID)
	fmt.Printf("Sending: %x\n", connectRequestBytes)
	_, err = conn.Write(connectRequestBytes)
	if err != nil {
		panic(err)
	}
	responseBuffer := make([]byte, 128)
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	n, _, err := conn.ReadFromUDP(responseBuffer)
	if err != nil {
		panic(err)
	}
	if n < 16 {
		panic(fmt.Errorf("less than 16 bytes received: %x", responseBuffer[:n]))
	}
	fmt.Printf("Got response: %x\n", responseBuffer[0:n])
	responseAction := binary.BigEndian.Uint32(responseBuffer[0:4])
	responseTransactionID := binary.BigEndian.Uint32(responseBuffer[4:8])
	responseConnectionID := binary.BigEndian.Uint64(responseBuffer[8:16])

	if responseTransactionID != transactionID {
		panic("udp tracker connect transaction ID mismatch")
	}
	if responseAction != 0 { // 0 => connect
		panic("udp tracker connect action not connect")
	}
	fmt.Printf("Got connection id: %d\n", responseConnectionID)
	fmt.Println("Done")
}
