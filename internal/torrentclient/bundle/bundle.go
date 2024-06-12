package bundle

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

type BundleFile struct {
	Path string
	Length int64
	ByteStart int64
	ByteEnd int64
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
	BitField *bitfield.BitField
	Complete bool
	mux sync.Mutex
}

func (bundle *Bundle) CheckComplete() error {
	bundle.mux.Lock()
	defer bundle.mux.Unlock()

	for _, piece := range bundle.Pieces {
		if !piece.Complete {
			return nil
		}
	}
	bundle.Complete = true
	return nil
}

func Create(path string, metaData *torrentfile.TorrentFile) (*Bundle, error) {
	bundle := Bundle{Path: path, Name: metaData.Info.Name, Complete: false, PieceLength: metaData.Info.PieceLength}
	if metaData.Info.Length != 0 {
		bundle.MultiFile = false
		bundle.Length = metaData.Info.Length
		bundle.File = &BundleFile{Path: fmt.Sprintf("%s/%s", path, metaData.Info.Name), Length: metaData.Info.Length}
	} else {
		bundle.MultiFile = true
		bundle.Files = make([]*BundleFile, len(metaData.Info.Files))
		for i, file := range metaData.Info.Files {
			bundle.Files[i] = &BundleFile{Length: file.Length}
			bundle.Files[i].Path = path
			for _, pathPiece := range file.Path {
				if i != len(file.Path) - 1 {
					err := os.MkdirAll(bundle.Files[i].Path, 0755)
					if err != nil {
						return nil, err
					}
				}
				bundle.Files[i].Path += fmt.Sprintf("/%s", pathPiece)
			}
		}
	}
	fmt.Println("Done creating dirs")
	bundle.NumPieces = len(metaData.Info.Pieces)
	fmt.Printf("Num pieces: %d\n", bundle.NumPieces)
	bundle.BitField = bitfield.New(bundle.NumPieces)
	bundle.Pieces = make([]*Piece, bundle.NumPieces)
	var err error
	pieceIndex := 0
	for _, fileDetails := range bundle.Files {
		curLen := int64(0)
		for curLen < fileDetails.Length {
			pieceLength := bundle.PieceLength
			if fileDetails.Length - bundle.PieceLength - int64(curLen) < 0 {
				pieceLength = fileDetails.Length - curLen
			}
			bundle.Pieces[pieceIndex], err = NewPiece(int64(pieceLength), metaData.Info.Pieces[pieceIndex])
			if err != nil {
				return nil, err
			}
			fmt.Printf("Added piece: %d / %d\n", pieceIndex, bundle.NumPieces)
			curLen += pieceLength
			pieceIndex++
		}
		fmt.Printf("Writing file: %s...\n", fileDetails.Path)
		if !checkFile(fileDetails.Path, fileDetails.Length) {
			file, err := os.OpenFile(fileDetails.Path, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return nil, err
			}
			for i := int64(0); i < fileDetails.Length; i++ {
				_, err := file.Write([]byte{0})
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return &bundle, nil
}

func checkFile(path string, length int64) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.Size() != length {
		return false
	}
	return true
}

func (bundle *Bundle) Delete() error {
	bundle.mux.Lock()
	defer bundle.mux.Unlock()

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

func (bundle *Bundle) getFile(byteOffset int64) (*BundleFile, error) {
	for _, file := range bundle.Files {
		if file.ByteStart >= byteOffset && file.ByteEnd <= byteOffset {
			return file, nil
		}
	}
	return nil, errors.New("byte not in files length")
}

func (bundle *Bundle) WriteBlock(pieceIndex int64, beginOffset int64, block []byte) error {
	bundle.mux.Lock()
	defer bundle.mux.Unlock()

	byteOffset := int64(0)
	for i := int64(0); i < pieceIndex; i++ {
		byteOffset += bundle.Pieces[i].Length
	}
	byteOffset += beginOffset

	blockOffset := 0
	for _, file := range bundle.Files {
		if byteOffset < file.Length {
			f, err := os.OpenFile(file.Path, os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = f.Seek(byteOffset, 0)
			if err != nil {
				return err
			}

			writeLen := len(block) - blockOffset
			if writeLen > int(file.Length - byteOffset) {
				writeLen = int(file.Length - byteOffset)
			}

			_, err = f.Write(block[blockOffset : blockOffset + writeLen])
			if err != nil {
				return err
			}

			blockOffset += writeLen
			byteOffset = 0

			if blockOffset == len(block) {
				break
			}
		} else {
			byteOffset -= file.Length
		}
	}

	//Update piece bitfield

	bundle.Pieces[pieceIndex].SetBlockWritten(byteOffset)

	bundle.Pieces[pieceIndex].CheckFull()
	if bundle.Pieces[pieceIndex].Full {
		hashCorrect, err := bundle.CheckPieceHash(pieceIndex)
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
	bundle.mux.Lock()
	defer bundle.mux.Unlock()

	block, err := bundle.readBlock(pieceIndex, beginOffset, length)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func (bundle *Bundle) readBlock(pieceIndex int64, beginOffset int64, length int64) ([]byte, error) {

	block := []byte{}

	byteOffset := int64(0)
	for i := int64(0); i < pieceIndex; i++ {
		byteOffset += bundle.Pieces[i].Length
	}
	byteOffset += beginOffset

	blockOffset := int64(0)
	for _, file := range bundle.Files {
		if byteOffset < file.Length {
			f, err := os.OpenFile(file.Path, os.O_RDONLY, 0644)
			if err != nil {
				return nil, err
			}
			defer f.Close()

			readLen := length - blockOffset
			if readLen > file.Length - byteOffset {
				readLen = file.Length - byteOffset
			}

			//Read
			buf := make([]byte, readLen)

			f.ReadAt(buf, byteOffset)
			if err != nil {
				return nil, err
			}

			block = append(block, buf...)

			blockOffset += readLen
			byteOffset = 0

			if blockOffset == length {
				break
			}
		} else {
			byteOffset -= file.Length
		}
	}
	return block, nil
}

func (bundle *Bundle) CheckPieceHash(pieceIndex int64) (bool, error) {
	bundle.mux.Lock()
	defer bundle.mux.Unlock()


	return true, nil
}