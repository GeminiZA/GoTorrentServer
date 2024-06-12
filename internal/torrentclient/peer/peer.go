package peer

import (
	"net"
)

type Peer struct {
	Interested bool
	Choked bool
	PeerChoking bool
	PeerInterested bool
	Has         []byte
	InfoHash    string
	PeerID      string
	conn		net.Conn
}

func New() (*Peer, error) {
	peer := Peer{Interested: false, Choked: true, PeerChoking: true, PeerInterested: false, }
	return &peer, nil
}

func Connect() error {
	return nil
}


func NewFormIn(conn net.Conn)