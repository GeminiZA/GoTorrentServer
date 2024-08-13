package message

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/GeminiZA/GoTorrentServer/internal/debugopts"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
)

const READ_TIMEOUT_MS = 500

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
	PORT // for DHT
	HANDSHAKE
)

type Message struct {
	Type     MessageType
	Index    uint32
	Length   uint32
	BitField []byte
	Begin    uint32
	Piece    []byte
	Port     uint32
	PeerID   []byte
	InfoHash []byte
	Reserved []byte
}

func (m *Message) GetBytes() []byte {
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
		len := uint32(1) + uint32(len(m.BitField))
		ret := make([]byte, 4)
		binary.BigEndian.PutUint32(ret, len)
		ret = append(ret, 5)
		ret = append(ret, m.BitField...)
		return ret
	case REQUEST:
		ret := make([]byte, 4)
		binary.BigEndian.PutUint32(ret, 13)
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
		ret := []byte{0, 0, 0, 3, 9}
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

func ParseHandshake(bytes []byte) (*Message, error) {
	msg := Message{Type: HANDSHAKE}
	if len(bytes) != 68 {
		return nil, errors.New("invalid handshake length")
	}
	pstrlen := bytes[0]
	if pstrlen != 19 {
		return nil, errors.New("invalid protocol string")
	}
	pstr := string(bytes[1:20])
	if pstr != "BitTorrent protocol" {
		return nil, errors.New("invalid protocol string")
	}
	if debugopts.MESSAGE_DEBUG {
		handshakeBytes := make([]byte, 0)
		handshakeBytes = append(handshakeBytes, pstrlen)
		handshakeBytes = append(handshakeBytes, []byte(pstr)...)
		handshakeBytes = append(handshakeBytes, bytes[20:28]...)
		handshakeBytes = append(handshakeBytes, bytes[28:48]...)
		handshakeBytes = append(handshakeBytes, bytes[48:68]...)
	}
	msg.Reserved = bytes[20:28]
	msg.InfoHash = bytes[28:48]
	msg.PeerID = bytes[48:68]
	return &msg, nil
}

func ParseMessage(msgBytes []byte) (*Message, error) {
	const MAX_MESSAGE_LENGTH = 17 * 1024

	msgLen := len(msgBytes)
	if msgLen > MAX_MESSAGE_LENGTH {
		return nil, errors.New("message exceeds max length")
	}
	if msgLen == 0 {
		return &Message{Type: KEEP_ALIVE}, nil
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
		bitField := msgBytes[1:msgLen]
		bitFieldLength := msgLen - 1 // In bytes
		return &Message{Type: BITFIELD, BitField: bitField, Length: uint32(bitFieldLength)}, nil
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

func (msg *Message) Print() string {
	ret := ""
	switch msg.Type {
	case KEEP_ALIVE:
		ret += "{Type: keep alive}"
	case CHOKE:
		ret += "{Type: choke}"
	case UNCHOKE:
		ret += "{Type: unchoke}"
	case INTERESTED:
		ret += "{Type: interested}"
	case NOT_INTERESTED:
		ret += "{Type: not interested}"
	case HAVE:
		ret += fmt.Sprintf("{Type: have, Index: %d}\n", msg.Index)
	case BITFIELD:
		s := fmt.Sprintf("{Type: bitfield, length: %d, field: blob}\n", msg.Length)
		s += "Bitfield:\n"
		for _, b := range msg.BitField {
			s += fmt.Sprintf("%08b ", b)
		}
		ret += s
	case REQUEST:
		ret += fmt.Sprintf("{Type: request, index: %d, begin: %d, length: %d}\n", msg.Index, msg.Begin, msg.Length)
	case PIECE:
		ret += fmt.Sprintf("{Type: piece, index: %d, begin: %d, length: %d}\n", msg.Index, msg.Begin, len(msg.Piece))
	case CANCEL:
		ret += fmt.Sprintf("{Type: cancel, index: %d, begin: %d, length: %d}\n", msg.Index, msg.Begin, msg.Length)
	case PORT:
		ret += fmt.Sprintf("{Type: port, port: %d}\n", msg.Port)
	case HANDSHAKE:
		ret += fmt.Sprintf("Type: handshake, peerID: %s\n", msg.PeerID)
	}
	return ret
}

func NewHandshake(infoHash []byte, peerID []byte) *Message {
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
	return &Message{Type: BITFIELD, BitField: bitfield, Length: uint32(len(bitfield))}
}

func NewRequest(bi *bundle.BlockInfo) *Message {
	return &Message{Type: REQUEST, Index: uint32(bi.PieceIndex), Begin: uint32(bi.BeginOffset), Length: uint32(bi.Length)}
}

func NewPiece(index uint32, beginOffset uint32, block []byte) *Message {
	return &Message{Type: PIECE, Index: index, Begin: beginOffset, Piece: block}
}

func NewCancel(bi *bundle.BlockInfo) *Message {
	return &Message{Type: CANCEL, Index: uint32(bi.PieceIndex), Begin: uint32(bi.BeginOffset), Length: uint32(bi.Length)}
}

func NewPort(port uint32) *Message {
	return &Message{Type: PORT, Port: port}
}
