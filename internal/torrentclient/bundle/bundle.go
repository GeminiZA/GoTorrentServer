package bundle

import (
	"os"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

type Bundle struct {
	PieceLength int64
	Length      int64
	Path string
	MetaData MetaFile
}


func (df *Bundle) Complete() error {
	return nil
}

func New(path string, length int64, pieceLength int64, metaData *torrentfile.TorrentFile) (*Bundle, error) {
	return nil, nil
}

func (df *Bundle) WriteBlock(pieceIndex int64, beginOffset int64, piece []byte) error {
	file, err := os.OpenFile(df.Path, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	offset := pieceIndex*df.PieceLength + beginOffset
	_, err = file.WriteAt(piece, offset)
	if err != nil {
		return err
	}

	return nil
}

func (df *Bundle) GetBlock(pieceIndex int64, beginOffset int64, length int64) ([]byte, error) {
	file, err := os.OpenFile(df.Path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	offset := pieceIndex*df.PieceLength + beginOffset
	_, err = file.Seek(offset, 0)
	if err != nil {
		return nil, err
	}
	buffer := make([]byte, length)
	_, err = file.Read(buffer)
	if err != nil {
		return nil, err
	}
	return buffer, nil
}

func (df *Bundle) Delete() error {
	err := os.Remove(df.Path)
	if err != nil {
		return err
	}
	return nil
}
