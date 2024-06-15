// Interface for only writing and storing pieces and.Blocks
package bundle

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
)

type Block struct {
	Length     int64
	Written    bool
	bytes 	   []byte
	Fetching   bool
}

type Piece struct {
	Blocks     []*Block
	Complete   bool
	hash 	   []byte
	length	   int64
	ByteOffset int64
}

func newBlock(length int64) *Block {
	return &Block{Length: length, Written: false, Fetching: false, bytes: nil}
}

func (block *Block) write(bytes []byte) {
	block.bytes = bytes
	block.Written = true
}

func NewPiece(length int64, pieceByteOffset int64, hash []byte) *Piece {
	const MAX_BLOCK_LENGTH int64 = 16384
	piece := Piece{length: length, hash: hash, ByteOffset: pieceByteOffset, Complete: false}
	curByte := int64(0)
	for curByte < length {
		curBlockLength := MAX_BLOCK_LENGTH
		if curByte + MAX_BLOCK_LENGTH > length {
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
	if BUNDLE_DEBUG {
		fmt.Printf("Found block to write: block index: %d\n", blockIndex)
	}
	piece.Blocks[blockIndex].write(bytes)
	if BUNDLE_DEBUG {
		fmt.Printf("Block Written successfully: block index: %d\n", blockIndex)
	}
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
			if BUNDLE_DEBUG {
				fmt.Println("Piece not full yet...")
			}
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