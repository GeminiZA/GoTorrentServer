package bundle

type Block struct {
	Length   int64
	Written  bool
	bytes    []byte
	Fetching bool
}

func newBlock(length int64) *Block {
	return &Block{Length: length, Written: false, Fetching: false, bytes: nil}
}

func (block *Block) write(bytes []byte) {
	block.bytes = bytes
	block.Written = true
}

type BlockInfo struct {
	PieceIndex  int64
	BeginOffset int64
	Length      int64
}