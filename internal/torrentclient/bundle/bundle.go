// Todo:
// load bundle from currently active torrent;
// read entire piece and store in LRU cache when client gets a block from it to upload;
// write files in new
package bundle

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

const BUNDLE_DEBUG bool = false

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
	BitField *bitfield.BitField
	Complete bool
	pieceCache *PieceCache
	mux sync.Mutex
}

func NewBundle(metaData *torrentfile.TorrentFile, bundlePath string, pieceCacheCapacity int) (*Bundle, error) {
	bundle := Bundle{
		Name: metaData.Info.Name,
		PieceLength: metaData.Info.PieceLength, 
		Complete: false, 
		Path: bundlePath,
		pieceCache: NewPieceCache(pieceCacheCapacity),
	}
	totalLength := int64(0)
	//Files
	if metaData.Info.Length == 0 {
		bundle.MultiFile = true
		curByte := int64(0)
		for _, file := range metaData.Info.Files {
			path := ""
			if len(bundle.Path) > 0 {
				path = fmt.Sprintf("./%s/%s", bundle.Path, metaData.Info.Name)
				err := os.MkdirAll(path, 0777)
				if err != nil {
					return nil, err
				}
			} else {
				path = fmt.Sprintf("./%s", metaData.Info.Name)
			}
			curBundleFile := BundleFile{ByteStart: curByte, Length: file.Length, Path: path}
			curByte += file.Length
			totalLength += file.Length
			for index, pathPiece := range file.Path {
				curBundleFile.Path += fmt.Sprintf("/%s", pathPiece)
				if index != len(file.Path) - 1 {
					err := os.MkdirAll(curBundleFile.Path, 0777)
					if err != nil {
						return nil, err
					}
				}
			}
			bundle.Files = append(bundle.Files, &curBundleFile)
		}
	} else { //Single File mode
		bundle.MultiFile = false
		bundleFile := &BundleFile{ByteStart: 0, Length: metaData.Info.Length, Path: fmt.Sprintf("./%s/%s", bundle.Path, metaData.Info.Name)}
		bundle.Files = []*BundleFile{bundleFile}
		totalLength = metaData.Info.Length
	}
	if BUNDLE_DEBUG {
		fmt.Println("Got files...")
		for _, file := range bundle.Files {
			fmt.Printf("Path: %s, Length: %d\n", file.Path, file.Length)
		}
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
		curByte += curPieceLength
	}
	bundle.NumPieces = len(bundle.Pieces)
	if BUNDLE_DEBUG {
		fmt.Printf("Got pieces (%d)\n", bundle.NumPieces)
		for i, piece := range bundle.Pieces {
			fmt.Printf("Piece %d hash: %v\n", i, piece.hash)
		}
	}

	//bitfield

	bundle.BitField = bitfield.New(bundle.NumPieces)

	//Write files

	for _, bundleFile := range bundle.Files {
		if !checkFile(bundleFile.Path, bundleFile.Length) {
			file, err := os.OpenFile(bundleFile.Path, os.O_WRONLY|os.O_CREATE, 0777)
			if err != nil {
				return nil, err
			}
			defer file.Close()

			for i := int64(0); i < bundleFile.Length; i++ {
				_, err = file.Write([]byte{0})
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
	return info.Size() == length
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
	bundle.mux.Lock()
	defer bundle.mux.Unlock()

	curPiece := bundle.Pieces[pieceIndex]
	bytes := curPiece.GetBytes()
	curByte := int64(0)
	for _, bundleFile := range bundle.Files {
		curByteOffset := curPiece.ByteOffset + curByte
		if bundleFile.ByteStart <= curByteOffset && curByteOffset <= bundleFile.ByteStart + bundleFile.Length {
			numBytesToWrite := int64(len(bytes)) - curByte
			fileBytesOffset := curByteOffset - bundleFile.ByteStart
			if numBytesToWrite + fileBytesOffset > bundleFile.Length {
				numBytesToWrite = bundleFile.Length - numBytesToWrite
			}
			file, err := os.OpenFile(bundleFile.Path, os.O_WRONLY, 0777)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = file.WriteAt(bytes[curByte : curByte + numBytesToWrite], fileBytesOffset)
			if err != nil {
				return err
			}

			curByte += numBytesToWrite
			if curByte == int64(len(bytes)) {
				break // Done writing piece
			}
		}
	}
	return nil
}

func (bundle *Bundle) loadPiece(pieceIndex int64) error {
	pieceBytes := []byte{}
	bytesLeft := bundle.Pieces[pieceIndex].length
	byteOffset := bundle.Pieces[pieceIndex].ByteOffset
	for _, bundleFile := range bundle.Files {
		if bundleFile.ByteStart <= byteOffset && bundleFile.ByteStart + bundleFile.Length <= byteOffset {
			fileByteOffset := byteOffset - bundleFile.ByteStart			
			readLen := bytesLeft
			if fileByteOffset + bytesLeft > bundleFile.Length {
				readLen = bytesLeft - (bundleFile.Length - fileByteOffset)
			}
			file, err := os.OpenFile(bundleFile.Path, os.O_RDONLY, 0777)
			if err != nil {
				return err
			}
			defer file.Close()
			buf := make([]byte, readLen)
			_, err = file.ReadAt(buf, fileByteOffset)
			if err != nil {
				return err
			}
			pieceBytes = append(pieceBytes, buf...)
			bytesLeft -= readLen
			byteOffset += readLen
			if bytesLeft == 0 {
				break
			}
		}
	}
	bundle.pieceCache.Put(pieceIndex, pieceBytes)
	return nil
}

func (bundle *Bundle) GetBlock(pieceIndex int64, beginOffset int64, length int64) ([]byte, error) {
	bundle.mux.Lock()
	defer bundle.mux.Unlock()
	
	if !bundle.pieceCache.Contains(pieceIndex) {
		bundle.loadPiece(pieceIndex)
	}
	pieceBytes := bundle.pieceCache.Get(pieceIndex)

	if length + beginOffset > bundle.Pieces[pieceIndex].length {
		return nil, errors.New("block overflows piece bounds")
	}

	return pieceBytes[beginOffset : length + beginOffset], nil
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
	for i := int64(0); i < 8; i++ {
		if !bundle.BitField.GetBit(nextByte * 8 + i) {
			pieceIndex = nextByte * 8 + i
			break
		}
	}
	beginOffset := int64(0)
	curByte := int64(0)
	for _, block := range bundle.Pieces[pieceIndex].blocks {
		if !block.written {
			beginOffset = curByte
			curByte += block.length
			break
		}
		curByte += block.length
	}
	length := curByte - beginOffset
	return pieceIndex, beginOffset, length, nil
}
