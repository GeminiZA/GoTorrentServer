// Interface for only writing and storing pieces and.Blocks
package bundle

import (
	"bytes"
	"crypto/sha1"
	"errors"
)

type Piece struct {
	Blocks     []*Block
	Complete   bool
	hash       []byte
	length     int64
	ByteOffset int64
}

func NewPiece(length int64, pieceByteOffset int64, hash []byte) *Piece {
	const MAX_BLOCK_LENGTH int64 = 16384
	piece := Piece{length: length, hash: hash, ByteOffset: pieceByteOffset, Complete: false}
	curByte := int64(0)
	for curByte < length {
		curBlockLength := MAX_BLOCK_LENGTH
		if curByte+MAX_BLOCK_LENGTH > length {
			curBlockLength = length - curByte
		}
		piece.Blocks = append(piece.Blocks, newBlock(curBlockLength))
		curByte += curBlockLength
	}
	return &piece
}

func (piece *Piece) Reset() {
	for _, block := range piece.Blocks {
		block.Written = false
		block.bytes = nil
	}
}

func (piece *Piece) WriteBlock(beginOffset int64, bytes []byte) error {
	curByte := int64(0)
	blockIndex := 0
	for i, block := range piece.Blocks {
		if curByte == beginOffset {
			blockIndex = i
			break
		}
		curByte += block.Length
	}
	if curByte == piece.length {
		return errors.New("could not find block")
	}
	if piece.Blocks[blockIndex].Written {
		return errors.New("block already written")
	}
	if piece.Blocks[blockIndex].Length != int64(len(bytes)) {
		return errors.New("block length incorrect")
	}
	piece.Blocks[blockIndex].write(bytes)
	if piece.checkFull() {
		correct, err := piece.checkHash()
		if err != nil {
			return err
		}
		if !correct {
			piece.Reset()
		} else {
			piece.Complete = true
		}
	}
	return nil
}

func (piece *Piece) checkHash() (bool, error) {
	hasher := sha1.New()
	for _, block := range piece.Blocks {
		_, err := hasher.Write(block.bytes)
		if err != nil {
			return false, err
		}
	}
	pieceHash := hasher.Sum(nil)
	return bytes.Equal(pieceHash, piece.hash), nil
}

func (piece *Piece) checkFull() bool {
	for _, block := range piece.Blocks {
		if !block.Written {
			return false
		}
	}
	return true
}

func (piece *Piece) GetBytes() []byte {
	pieceBytes := []byte{}
	for _, block := range piece.Blocks {
		pieceBytes = append(pieceBytes, block.bytes...)
	}
	return pieceBytes
}

func (piece *Piece) CancelBlock(beginOffset int64) error {
	curByte := int64(0)
	for _, block := range piece.Blocks {
		if curByte == beginOffset {
			block.Fetching = false
			return nil
		}
		curByte += block.Length
	}
	return errors.New("offset not found in blocks (CancelBlock)")
}
