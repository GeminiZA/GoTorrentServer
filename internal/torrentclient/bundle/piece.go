package bundle

import "errors"

type Block struct {
	ByteOffset int64
	Length int64
	Written    bool
}

type Piece struct {
	PieceIndex int64
	Blocks     []Block
}

func NewPiece(index int64, pieceLength int64) (*Piece, error) {
	piece := Piece{PieceIndex: index}
	numFullBlocks := pieceLength / 16384
	for i := 0; i < int(numFullBlocks); i++ {
		piece.Blocks = append(piece.Blocks, Block{
												ByteOffset: 16384*int64(i),
												Written: false,
												Length: 16384,
											})
	}
	if numFullBlocks * 16384 < pieceLength {
		lastBlockLength := pieceLength - (numFullBlocks * 16384)
		piece.Blocks = append(piece.Blocks, Block{
												ByteOffset: numFullBlocks * 16384,
												Written: false,
												Length: lastBlockLength,
											})
	}
	return &piece, nil
}

func (piece *Piece) WritePiece(path string) error {
	for _, block := range piece.Blocks {
		if !block.Written {
			return errors.New("piece not complete")
		}
	}
}