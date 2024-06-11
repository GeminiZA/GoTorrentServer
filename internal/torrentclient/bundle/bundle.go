package bundle

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"os"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

type BundleFile struct {
	Path string
	Length int64
	PieceStart int64
	PieceEnd int64
}

type Bundle struct {
	Name string
	PieceLength int64
	Length      int64
	Path string
	Pieces []*Piece
	NumPieces int
	Files []*BundleFile
	MultiFile bool
	File *BundleFile
	BitField []byte
	Complete bool
}

func (bundle *Bundle) GetBit(pieceIndex int64) (bool, error) {
	if pieceIndex > int64(len(bundle.BitField)) {
		return false, errors.New("out of range")
	}
	byteOffset := pieceIndex / 8
	bitOffset := pieceIndex % 8
	pieceByte := bundle.BitField[byteOffset]
	ret := (pieceByte >> uint(7-bitOffset)) & 1	
	return ret == 1, nil
}

func (bundle *Bundle) SetBit(pieceIndex int64) error {
	if pieceIndex > int64(len(bundle.BitField)) {
		return errors.New("out of range")
	}
	byteOffset := pieceIndex / 8
	bitOffset := pieceIndex % 8
	pieceByte := bundle.BitField[byteOffset]
	pieceByte = pieceByte | (1 << uint(7 - bitOffset))
	bundle.BitField[byteOffset] = pieceByte
	return nil
}

func (bundle *Bundle) CheckComplete() error {
	for _, piece := range bundle.Pieces {
		if !piece.Complete {
			return nil
		}
	}
	bundle.Complete = true
	return nil
}

func New(path string, metaData *torrentfile.TorrentFile) (*Bundle, error) {
	bundle := Bundle{Path: path, Name: metaData.Info.Name, Complete: false}
	if metaData.Info.Length != 0 {
		bundle.MultiFile = false
		bundle.Length = metaData.Info.Length
		bundle.File = &BundleFile{Path: fmt.Sprintf("%s/%s", path, metaData.Info.Name), Length: metaData.Info.Length, PieceStart: 0, PieceEnd: int64(len(metaData.Info.Pieces))}
	} else {
		bundle.MultiFile = true
		bundle.Files = make([]*BundleFile, len(metaData.Info.Files))
		curPieceIndex := int64(0)
		for i, file := range metaData.Info.Files {
			bundle.Files[i].Length = file.Length
			bundle.Files[i].Path = path
			for _, pathPiece := range file.Path {
				bundle.Files[i].Path += fmt.Sprintf("/%s", pathPiece)
			}
			numPieces := (file.Length + metaData.Info.PieceLength - 1) / metaData.Info.PieceLength
			bundle.Files[i].PieceStart = curPieceIndex
			bundle.Files[i].PieceEnd = curPieceIndex + numPieces
		}
	}
	bundle.NumPieces = len(metaData.Info.Pieces)
	bundle.Pieces = make([]*Piece, bundle.NumPieces)
	for i := 0; i < len(metaData.Info.Pieces); i++ {
		bundle.Pieces[i].hash = metaData.Info.Pieces[i]
	}
	return nil, nil
}

func (bundle *Bundle) Delete() error {
	var err error
	if bundle.MultiFile {
		for _, file := range bundle.Files {
			err = os.Remove(file.Path)
			if err != nil {
				return err
			}
		}
	} else {
		err = os.Remove(bundle.Path)
		if err != nil {
			return err
		}
	}
	return nil
}

func (bundle *Bundle) GetFile(pieceIndex int64) (*BundleFile, error) {
	if bundle.MultiFile {
		for _, file := range bundle.Files {
			if file.PieceStart < pieceIndex && file.PieceEnd <= pieceIndex {
				return file, nil
			}
		}
	} else {
		return bundle.File, nil
	}
	return nil, errors.New("file not found")
}

func (bundle *Bundle) WriteBlock(pieceIndex int64, beginOffset int64, block []byte) error {
	curFile, err := bundle.GetFile(pieceIndex)
	if err != nil {
		return errors.New("trying to write block to out of bounds piece")
	}
	file, err := os.OpenFile(curFile.Path, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	byteOffset := int64(0)
	for i := curFile.PieceStart; i < pieceIndex; i++ {
		byteOffset += bundle.Pieces[i].Length
	}
	byteOffset += beginOffset

	_, err = file.Seek(byteOffset, 0)
	if err != nil {
		return err
	}

	_, err = file.Write(block)
	if err != nil {
		return err
	}
	//Update piece bitfield

	bundle.Pieces[pieceIndex].SetBlockWritten(byteOffset)

	bundle.Pieces[pieceIndex].CheckFull()
	if bundle.Pieces[pieceIndex].Full {
		hashCorrect, err := bundle.CheckHash(pieceIndex)
		if err != nil {
			return nil
		}
		if hashCorrect {
			bundle.Pieces[pieceIndex].Complete = true
		} else {
			bundle.Pieces[pieceIndex].Reset()
		}
	}

	return nil
}

func (bundle *Bundle) GetBlock(pieceIndex int64, beginOffset int64, length int64) ([]byte, error) {
	curFile, err := bundle.GetFile(pieceIndex)
	if err != nil {
		return nil, errors.New("trying to read block to out of bounds piece")
	}
	file, err := os.OpenFile(curFile.Path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	byteOffset := int64(0)
	for i := curFile.PieceStart; i < pieceIndex; i++ {
		byteOffset += bundle.Pieces[i].Length
	}
	byteOffset += beginOffset

	blockIsWritten, err := bundle.Pieces[pieceIndex].IsBlockWritten(byteOffset)
	if err != nil {
		return nil, err
	}
	if !blockIsWritten {
		return nil, errors.New("getting block that isnt written")
	}

	block := make([]byte, length)
	_, err = file.ReadAt(block, byteOffset)
	if err != nil {
		return nil, err
	}

	return block, nil
}

func (bundle *Bundle) CheckHash(pieceIndex int64) (bool, error) {
	curFile, err := bundle.GetFile(pieceIndex)
	if err != nil {
		return false, errors.New("trying to read block to out of bounds piece")
	}
	file, err := os.OpenFile(curFile.Path, os.O_RDONLY, 0644)
	if err != nil {
		return false, err
	}
	defer file.Close()
	
	byteOffset := int64(0)
	for i := curFile.PieceStart; i < pieceIndex; i++ {
		byteOffset += bundle.Pieces[i].Length
	}

	pieceBytes := make([]byte, bundle.Pieces[pieceIndex].Length)

	_, err = file.ReadAt(pieceBytes, byteOffset)
	if err != nil {
		return false, err
	}

	hasher := sha1.New()
	_, err = hasher.Write(pieceBytes)
	if err != nil {
		return false, err
	}

	pieceHash := hasher.Sum(nil)

	for i, curByte := range bundle.Pieces[pieceIndex].hash {
		if pieceHash[i] != curByte {
			return false, nil
		}
	}

	return true, nil
}