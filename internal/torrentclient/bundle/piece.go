package bundle

import "errors"

type Block struct {
	ByteOffset int64
	Length int64
	Written    bool
}

type Piece struct {
	Blocks     []Block
	Complete   bool
	hash 	   []byte
	Length	   int64
	Full	   bool
}

func NewPiece(pieceLength int64, hash []byte) (*Piece, error) {
	const MAX_BLOCK_SIZE int64 = 16384
	piece := Piece{Full: false, Complete: false}
	numFullBlocks := pieceLength / MAX_BLOCK_SIZE
	for i := 0; i < int(numFullBlocks); i++ {
		piece.Blocks = append(piece.Blocks, Block{
												ByteOffset: MAX_BLOCK_SIZE*int64(i),
												Written: false,
												Length: MAX_BLOCK_SIZE,
											})
	}
	if numFullBlocks * MAX_BLOCK_SIZE < pieceLength {
		lastBlockLength := pieceLength - (numFullBlocks * MAX_BLOCK_SIZE)
		piece.Blocks = append(piece.Blocks, Block{
												ByteOffset: numFullBlocks * MAX_BLOCK_SIZE,
												Written: false,
												Length: lastBlockLength,
											})
	}
	return &piece, nil
}

func (piece *Piece) IsBlockWritten(byteOffset int64) (bool, error) {
	for _, block := range piece.Blocks {
		if block.ByteOffset > byteOffset {
			return false, errors.New("invalid block byte offset")
		} else if block.ByteOffset == byteOffset {
			return block.Written, nil
		}
	}
	return false, nil
}

func (piece *Piece) SetBlockWritten(byteOffset int64) error {
	for _, block := range piece.Blocks {
		if block.ByteOffset > byteOffset {
			return errors.New("invalid block byte offset")
		} else if block.ByteOffset == byteOffset {
			block.Written = true
			return nil
		}
	}
	return nil
}

func (piece *Piece) Reset() {
	for _, block := range piece.Blocks {
		block.Written = false
	}
	piece.Full = false
}

func (piece *Piece) CheckFull() {
	for _, block := range piece.Blocks {
		if !block.Written {
			return
		}
	}
	piece.Full = true
}