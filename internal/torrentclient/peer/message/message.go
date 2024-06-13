package message

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

type MessageType int

const (
	KEEP_ALIVE MessageType = iota
	CHOKE
	UNCHOKE
	INTERESTED
	NOT_INTERESTED
	HAVE
	BITFIELD
	REQUEST
	PIECE
	CANCEL
	PORT //for DHT
	HANDSHAKE
)

type Message struct {
	Type MessageType
	Index uint32
	Length uint32
	BitField []byte
	Begin uint32
	Piece []byte
	Port uint32
	PeerID string
	InfoHash []byte
	Reserved []byte
}

func (m Message) GetBytes() []byte {
	switch m.Type {
	case KEEP_ALIVE:
		return []byte{0, 0, 0, 0}
	case CHOKE:
		return []byte{0, 0, 0, 1, 0}
	case UNCHOKE:
		return []byte{0, 0, 0, 1, 1}
	case INTERESTED:
		return []byte{0, 0, 0, 1, 2}
	case NOT_INTERESTED:
		return []byte{0, 0, 0, 1, 3}
	case HAVE:
		index := make([]byte, 4)
		binary.BigEndian.PutUint32(index, m.Index)
		ret := []byte{0, 0, 0, 5, 4}
		ret = append(ret, index...)
		return ret
	case BITFIELD:
		len := uint32(1) + m.Length
		ret := make([]byte, 4)
		binary.BigEndian.PutUint32(ret, len)
		ret = append(ret, 5)
		ret = append(ret, m.BitField...)
		return ret
	case REQUEST:
		ret := make([]byte, 4)
		binary.BigEndian.PutUint32(ret, 13)
		fmt.Printf("RET: %v\n", ret)
		ret = append(ret, 6)
		partIndex := make([]byte, 4)
		binary.BigEndian.PutUint32(partIndex, m.Index)
		beginOffset := make([]byte, 4)
		binary.BigEndian.PutUint32(beginOffset, m.Begin)
		reqLength := make([]byte, 4)
		binary.BigEndian.PutUint32(reqLength, m.Length)
		ret = append(ret, partIndex...)
		ret = append(ret, beginOffset...)
		ret = append(ret, reqLength...)
		return ret
	case PIECE:
		len := 9 + uint32(len(m.Piece))
		lenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBytes, len)
		ret := []byte{}
		ret = append(ret, lenBytes...)
		ret = append(ret, 7)
		partIndex := make([]byte, 4)
		binary.BigEndian.PutUint32(partIndex, m.Index)
		beginOffset := make([]byte, 4)
		binary.BigEndian.PutUint32(beginOffset, m.Begin)
		ret = append(ret, partIndex...)
		ret = append(ret, beginOffset...)
		ret = append(ret, m.Piece...)
		return ret
	case CANCEL:
		ret := make([]byte, 4)
		binary.BigEndian.PutUint32(ret, 13)
		ret = append(ret, 8)
		partIndex := make([]byte, 4)
		binary.BigEndian.PutUint32(partIndex, m.Index)
		beginOffset := make([]byte, 4)
		binary.BigEndian.PutUint32(beginOffset, m.Begin)
		reqLength := make([]byte, 4)
		binary.BigEndian.PutUint32(reqLength, m.Length)
		ret = append(ret, partIndex...)
		ret = append(ret, beginOffset...)
		ret = append(ret, reqLength...)
		return ret
	case PORT:
		portBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(portBytes, m.Port)
		ret := []byte{0,0,0,3,9}
		ret = append(ret, portBytes...)
		return ret
	case HANDSHAKE:
		pstr := []byte("BitTorrent protocol")
		pstrlen := byte(19)
		reserved := make([]byte, 8)
		infoHash := []byte(m.InfoHash)
		peerID := []byte(m.PeerID)
		ret := []byte{pstrlen}
		ret = append(ret, pstr...)
		ret = append(ret, reserved...)
		ret = append(ret, infoHash...)
		ret = append(ret, peerID...)
		return ret
	}
	return nil
}

func ReadHandshake(conn net.Conn, timeout time.Duration) (*Message, error) {
	msg := Message{Type: HANDSHAKE}
	deadline := time.Now().Add(timeout)
	pstrlenBytes := make([]byte, 1)
	err := conn.SetReadDeadline(deadline)
	if err != nil {
		return nil, err
	}
	_, err = conn.Read(pstrlenBytes)
	if err != nil {
		return nil, err
	}
	pstrlen := pstrlenBytes[0]
	if pstrlen != 19 {
		return nil, errors.New("invalid protocol string")
	}
	restLen := pstrlen + 48 // reserved (8) infohash (20) peerID (20)
	rest := make([]byte, restLen) 
	_, err = conn.Read(rest)
	if err != nil {
		return nil, err
	}
	if string(rest[0:pstrlen]) != "BitTorrent protocol" {
		return nil, errors.New("invalid protocol string")
	}
	msg.Reserved = rest[pstrlen : pstrlen + 8]
	msg.InfoHash = rest[pstrlen + 8:pstrlen + 28]
	msg.PeerID = string(rest[pstrlen + 28: pstrlen + 48])
	return &msg, nil
}

func ReadMessage(conn net.Conn) (*Message, error) {
	const MAX_MESSAGE_LENGTH = 17 * 1024
	msgLenBytes := make([]byte, 4)
	_, err := conn.Read(msgLenBytes)
	if err != nil {
		return nil, err
	}
	msgLen := binary.BigEndian.Uint32(msgLenBytes)
	if msgLen > MAX_MESSAGE_LENGTH {
		return nil, errors.New("message exceeds max length")
	}
	if msgLen == 0 {
		return &Message{Type: KEEP_ALIVE}, nil
	}
	msgBytes := make([]byte, 0)
	bytesRead := 0
	for bytesRead < int(msgLen) {
		buf := make([]byte, msgLen)
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		msgBytes = append(msgBytes, buf[:n]...)
		bytesRead += n
	}
	msgID := msgBytes[0]
	switch msgID {
	case 0: // choke
		return &Message{Type: CHOKE}, nil
	case 1: // unchoke
		return &Message{Type: UNCHOKE}, nil
	case 2: // interested
		return &Message{Type: INTERESTED}, nil
	case 3: // not interested
		return &Message{Type: NOT_INTERESTED}, nil
	case 4: // have
		pieceIndex := binary.BigEndian.Uint32(msgBytes[1:5])
		return &Message{Type: HAVE, Index: pieceIndex}, nil
	case 5: // bitfield
		bitField := msgBytes[5:msgLen]
		return &Message{Type: BITFIELD, BitField: bitField}, nil
	case 6: // request
		pieceIndex := binary.BigEndian.Uint32(msgBytes[1:5])
		beginOffset := binary.BigEndian.Uint32(msgBytes[5:9])
		blockLength := binary.BigEndian.Uint32(msgBytes[9:13])
		return &Message{Type: REQUEST, Index: pieceIndex, Begin: beginOffset, Length: blockLength}, nil
	case 7: // piece
		pieceIndex := binary.BigEndian.Uint32(msgBytes[1:5])
		beginOffset := binary.BigEndian.Uint32(msgBytes[5:9])
		block := msgBytes[9:msgLen]
		return &Message{Type: PIECE, Index: pieceIndex, Begin: beginOffset, Piece: block}, nil
	case 8: // cancel
		pieceIndex := binary.BigEndian.Uint32(msgBytes[1:5])
		beginOffset := binary.BigEndian.Uint32(msgBytes[5:9])
		blockLength := binary.BigEndian.Uint32(msgBytes[9:13])
		return &Message{Type: CANCEL, Index: pieceIndex, Begin: beginOffset, Length: blockLength}, nil
	case 9: // port
		port := binary.BigEndian.Uint32(msgBytes[5:9])
		return &Message{Type: PORT, Port: port}, nil
	}
	return nil, errors.New("invalid message id")
}

func (msg *Message) Print() {
	switch msg.Type {
	case KEEP_ALIVE:
		fmt.Println("{Type: keep alive}")
	case CHOKE:
		fmt.Println("{Type: choke}")
	case UNCHOKE:
		fmt.Println("{Type: unchoke}")
	case INTERESTED:
		fmt.Println("{Type: interested}")
	case NOT_INTERESTED:
		fmt.Println("{Type: not interested}")
	case HAVE:
		fmt.Printf("{Type: have, Index: %d}\n", msg.Index)
	case BITFIELD:
		fmt.Printf("{Type: bitfield, field: blob}\n")
	case REQUEST:
		fmt.Printf("{Type: request, index: %d, begin: %d, length: %d}\n", msg.Index, msg.Begin, msg.Length)
	case PIECE:
		fmt.Printf("{Type: piece, index: %d, begin: %d}\n", msg.Index, msg.Begin)
	case CANCEL:
		fmt.Printf("{Type: cancel, index: %d, begin: %d, length: %d}\n", msg.Index, msg.Begin, msg.Length)
	case PORT:
		fmt.Printf("{Type: port, port: %d}\n", msg.Port)
	case HANDSHAKE:
		fmt.Printf("Type: handshake, peerID: %s", msg.PeerID)
	}
}

func NewHandshake(infoHash []byte, peerID string) *Message {
	msg := Message{Type: HANDSHAKE}
	msg.InfoHash = infoHash
	msg.PeerID = peerID
	return &msg
}

func NewKeepAlive() *Message {
	return &Message{Type: KEEP_ALIVE}
}

func NewChoke() *Message {
	return &Message{Type: CHOKE}
}

func NewUnchoke() *Message {
	return &Message{Type: UNCHOKE}
}

func NewInterested() *Message {
	return &Message{Type: INTERESTED}
}

func NewNotInterested() *Message {
	return &Message{Type: NOT_INTERESTED}
}

func NewHave(index uint32) *Message {
	return &Message{Type: HAVE, Index: index}
}

func NewBitfield(bitfield []byte) *Message {
	return &Message{Type: BITFIELD, BitField: bitfield}
}

func NewRequest(index int64, beginOffset int64, length int64) *Message {
	return &Message{Type: REQUEST, Index: uint32(index), Begin: uint32(beginOffset), Length: uint32(length)}
}

func NewPiece(index uint32, beginOffset uint32, block []byte) *Message {
	return &Message{Type: PIECE, Index: index, Begin: beginOffset, Piece: block}
}

func NewCancel(index uint32, beginOffset uint32, length uint32) *Message {
	return &Message{Type: CANCEL, Index: index, Begin: beginOffset, Length: length}
}

func NewPort(port uint32) *Message {
	return &Message{Type: PORT, Port: port}
}