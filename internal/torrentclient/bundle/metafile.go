package bundle

import (
	"errors"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

type MetaFile struct {
	BitField []byte
	MultiFile bool
	Files []torrentfile.FileInfo
	Length int64
	Data     *torrentfile.TorrentFile
}

func CreateMetaFile(tf *torrentfile.TorrentFile) (*MetaFile, error) {
	mf := MetaFile{Data: tf}
	if tf.Info.Length != 0 {
		mf.MultiFile = true
		mf.Files = tf.Info.Files
		tot := 0
		for i := range mf.Files {
			tot += int(mf.Files[i].Length)
		}
		mf.Length = int64(tot)
	} else {
		mf.MultiFile = false
		mf.Length = tf.Info.Length
	}
	return nil, nil
}

func LoadMetaFile(path string) (*MetaFile, error) {

	return nil, nil
}

func (mf *MetaFile) GetBit(pieceIndex int64) (bool, error) {
	if pieceIndex > int64(len(mf.BitField)) {
		return false, errors.New("out of range")
	}
	byteOffset := pieceIndex / 8
	bitOffset := pieceIndex % 8
	pieceByte := mf.BitField[byteOffset]
	ret := (pieceByte >> uint(7-bitOffset)) & 1	
	return ret == 1, nil
}

func (mf *MetaFile) SetBit(pieceIndex int64) error {
	if pieceIndex > int64(len(mf.BitField)) {
		return errors.New("out of range")
	}
	byteOffset := pieceIndex / 8
	bitOffset := pieceIndex % 8
	pieceByte := mf.BitField[byteOffset]
	pieceByte = pieceByte | (1 << uint(7 - bitOffset))
	mf.BitField[byteOffset] = pieceByte
	return nil
}