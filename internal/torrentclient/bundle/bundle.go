// Todo: load bundle from currently active torrent
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

func NewBundle(metaData *torrentfile.TorrentFile, bundlePath string) (*Bundle, error) {
	bundle := Bundle{Name: metaData.Info.Name, PieceLength: metaData.Info.PieceLength, Complete: false, Path: bundlePath}
	totalLength := int64(0)
	//Files
	if metaData.Info.Length == 0 {
		bundle.MultiFile = true
		curByte := int64(0)
		for _, file := range metaData.Info.Files {
			curBundleFile := BundleFile{ByteStart: curByte, Length: file.Length, Path: fmt.Sprintf("./%s/%s", bundle.Path, metaData.Info.Name)}
			curByte += file.Length
			totalLength += file.Length
			for _, pathPiece := range file.Path {
				curBundleFile.Path += pathPiece
			}
			bundle.Files = append(bundle.Files, &curBundleFile)
		}
	} else { //Single File mode
		bundle.MultiFile = false
		bundle.File = &BundleFile{ByteStart: 0, Length: metaData.Info.Length, Path: fmt.Sprintf("./%s/%s", bundle.Path, metaData.Info.Name)}
		totalLength = metaData.Info.Length
	}
	if metaData.Info.PieceLength == 0 {
		return nil, errors.New("no piece length")
	}
	//Pieces
	if len(metaData.Info.Pieces) == 0 {
		return nil, errors.New("no pieces")
	}
	curByte := int64(0)
	bundle.PieceLength = metaData.Info.PieceLength
	for _, pieceHash := range metaData.Info.Pieces {
		curPieceLength := bundle.PieceLength
		if curByte + bundle.PieceLength > totalLength {
			curPieceLength = totalLength - curByte
		}
		curPiece := NewPiece(curPieceLength, curByte, pieceHash)
		bundle.Pieces = append(bundle.Pieces, curPiece)
	}
	bundle.NumPieces = len(bundle.Pieces)

	//bitfield

	bundle.BitField = bitfield.New(bundle.NumPieces)
	
	return &bundle, nil
}

func (bundle *Bundle) WriteBlock(pieceIndex int64, beginOffset int64, block []byte) error { //beginOffset will always match a block in pieces
	bundle.mux.Lock()
	defer bundle.mux.Unlock()
	
	err := bundle.Pieces[pieceIndex].WriteBlock(beginOffset, block)
	if err != nil {
		return err
	}
	if bundle.Pieces[pieceIndex].Complete() {
		bundle.BitField.SetBit(pieceIndex)
		err := bundle.WritePiece(pieceIndex)
		if err != nil {
			return err
		}
	}
	if bundle.BitField.Complete() {
		bundle.Complete = true
	}
	return err
}

func (bundle *Bundle) WritePiece(pieceIndex int64) error {
	curPiece := bundle.Pieces[pieceIndex]
	bytes := curPiece.GetBytes()
	curByte := int64(0)
	fileStartByteOffset := int64(0)
	for _, bundleFile := range bundle.Files {
		curByteOffset := curPiece.ByteOffset + curByte
		if bundleFile.ByteStart <= curByteOffset && curByteOffset <= bundleFile.ByteStart + bundleFile.Length {
			numBytesToWrite := len(bytes) - int(curByte)
			
		}
		fileStartByteOffset += bundleFile.Length
	}
}

func (bundle *Bundle) GetBlock(pieceIndex int64, beginOffset int64, length int64) ([]byte, error) {
	bundle.mux.Lock()
	defer bundle.mux.Unlock()
	
	//Read block from file directly
	block := []byte{}
	bytesLeft := length
	byteOffset := bundle.Pieces[pieceIndex].ByteOffset + beginOffset
	for _, bundleFile := range bundle.Files {
		if bundleFile.ByteStart <= byteOffset && bundleFile.ByteStart + bundleFile.Length <= byteOffset {
			fileByteOffset := byteOffset - bundleFile.ByteStart			
			readLen := bytesLeft
			if fileByteOffset + bytesLeft > bundleFile.Length {
				readLen = bytesLeft - (bundleFile.Length - fileByteOffset)
			}
			file, err := os.OpenFile(bundleFile.Path, os.O_RDONLY, 0644)
			if err != nil {
				return nil, err
			}
			defer file.Close()
			buf := make([]byte, readLen)
			_, err = file.ReadAt(buf, fileByteOffset)
			if err != nil {
				return nil, err
			}
			bytesLeft -= readLen
			byteOffset += readLen
			if bytesLeft == 0 {
				break
			}
		}
	}
	return block, nil
}

func (bundle *Bundle) NextBlock() (int64, int64, int64, error) { // Returns pieceIndex, beginOffset, blockLength
	bundle.mux.Lock()
	defer bundle.mux.Unlock()
	
	nextByte := int64(0)
	pieceIndex := int64(0)
	for i, b := range bundle.BitField.Bytes {
		if b != 0xFF {
			nextByte = int64(i)
			break
		}
	}
	for i := 0; i < 8; i++ {
		if !bundle.BitField.GetBit(int64(nextByte) + int64(i)) {
			pieceIndex = nextByte + int64(i)
			break
		}
	}
	beginOffset := int64(0)
	curByte := int64(0)
	for _, block := range bundle.Pieces[pieceIndex].blocks {
		if !block.written {
			beginOffset = curByte
			break
		}
		curByte += block.length
	}
	length := curByte - beginOffset
	return pieceIndex, beginOffset, length, nil
}
