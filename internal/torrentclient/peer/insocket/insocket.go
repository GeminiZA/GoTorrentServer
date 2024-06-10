package insocket

import (
	"errors"
	"fmt"
	"net"
	"time"
)

type InSocket struct {
	Port int
	running bool
	run bool
	initialized bool
}

func New(port int) (*InSocket, error) {
	var is InSocket
	is.Port = port
	is.initialized = true
	is.run = false
	is.running = false
	return &is, nil
}

func (is *InSocket) IsRunning() bool {
	return is.running
}

func (is *InSocket) Listen() error  {
	fmt.Printf("in socket started on port: %d\n", is.Port)
	if !is.initialized {
		return errors.New("in socket not initialized")
	}
	is.run = true
	is.running = true
	ln, err := net.Listen("tcp", fmt.Sprintf("%d", is.Port))
	if err != nil {
		return err
	}
	defer ln.Close()
	for is.run {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err)
			continue
		}

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			fmt.Println("Error reading message: ", err)
			continue
		}

		fmt.Println("Received message: ", string(buf[:n]))
		
		conn.Close()
	}
	is.running = false

	return nil
}

func (is *InSocket) Close() error {
	is.run = false
	i := 0
	for is.running && i < 100 {
		time.Sleep(100 * time.Millisecond)
		i++
	}
	if i == 100 {
		return errors.New("cannot close in socket")
	}
	return nil
}