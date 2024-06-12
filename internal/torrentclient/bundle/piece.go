// Interface for only writing and storing pieces and blocks
package bundle

import (
	"bytes"
	"crypto/sha1"
	"errors"
)

type Block struct {
	length     int64
	written    bool
	bytes 	   []byte
}

type Piece struct {
	blocks     []*Block
	complete   bool
	hash 	   []byte
	length	   int64
	ByteOffset int64
}

func newBlock(length int64) *Block {
	return &Block{length: length, written: false}
}

func (block *Block) write(bytes []byte) {
	block.bytes = bytes
	block.written = true
}

func NewPiece(length int64, pieceByteOffset int64, hash []byte) *Piece {
	const MAX_BLOCK_LENGTH int64 = 16384
	piece := Piece{length: length, hash: hash, ByteOffset: pieceByteOffset, complete: false}
	curByte := int64(0)
	for curByte < length {
		curBlockLength := MAX_BLOCK_LENGTH
		if curByte + MAX_BLOCK_LENGTH > length {
			curBlockLength = length - curByte
		}
		piece.blocks = append(piece.blocks, newBlock(curBlockLength))
	}
	return &piece
}

func (piece *Piece) Reset() {
	for _, block := range piece.blocks {
		block.written = false
		block.bytes = nil
	}
}

func (piece *Piece) Complete() bool {
	return piece.complete
}

func (piece *Piece) WriteBlock(beginOffset int64, bytes []byte) error {
	curByte := int64(0)
	blockIndex := 0
	for i, block := range piece.blocks {
		if curByte == beginOffset {
			blockIndex = i
			break
		}
		curByte += block.length
	}
	if curByte == piece.length {
		return errors.New("could not find block")
	}
	if piece.blocks[blockIndex].written {
		return errors.New("block already written")
	}
	if piece.blocks[blockIndex].length != int64(len(bytes)) {
		return errors.New("block length incorrect")
	}
	piece.blocks[blockIndex].write(bytes)
	if piece.checkFull() {
		correct, err := piece.checkHash()
		if err != nil {
			return err
		}
		if !correct {
			piece.Reset()
		} else {
			piece.complete = true
		}
	}
	return nil
}

func (piece *Piece) checkHash() (bool, error) {
	hasher := sha1.New()
	for _, block := range piece.blocks {
		_, err := hasher.Write(block.bytes)
		if err != nil {
			return false, err
		}
	}
	pieceHash := hasher.Sum(nil)
	return bytes.Equal(pieceHash, piece.hash), nil
}

func (piece *Piece) checkFull() bool {
	for _, block := range piece.blocks {
		if !block.written {
			return false
		}
	}
	return true
}

func (piece *Piece) GetBytes() []byte {
	pieceBytes := []byte{}
	for _, block := range piece.blocks {
		pieceBytes = append(pieceBytes, block.bytes...)
	}
	return pieceBytes
}