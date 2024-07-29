// Todo:
// load bundle from currently active torrent;
// read entire piece and store in LRU cache when client gets a block from it to upload;
// write files in new
// hash piece before writing
// something in here hangs when writing the piece
package bundle

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/GeminiZA/GoTorrentServer/internal/debugopts"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

type BundleFile struct {
	Path      string
	Length    int64
	ByteStart int64
}

type Bundle struct {
	Name        string
	InfoHash    []byte
	PieceLength int64
	Length      int64
	Path        string
	Pieces      []*Piece
	NumPieces   int64
	Files       []*BundleFile
	MultiFile   bool
	Bitfield    *bitfield.Bitfield
	Complete    bool
	pieceCache  *PieceCache
	mux         sync.Mutex
}

func NewBundle(metaData *torrentfile.TorrentFile, bundlePath string, pieceCacheCapacity int) (*Bundle, error) {
	bundle := Bundle{
		Name:        metaData.Info.Name,
		PieceLength: metaData.Info.PieceLength,
		Complete:    false,
		Path:        bundlePath,
		pieceCache:  NewPieceCache(pieceCacheCapacity),
		InfoHash:    metaData.InfoHash,
	}
	totalLength := int64(0)
	// Files
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
				if index != len(file.Path)-1 {
					err := os.MkdirAll(curBundleFile.Path, 0777)
					if err != nil {
						return nil, err
					}
				}
			}
			bundle.Files = append(bundle.Files, &curBundleFile)
		}
	} else { // Single File mode
		bundle.MultiFile = false
		bundleFile := &BundleFile{ByteStart: 0, Length: metaData.Info.Length, Path: fmt.Sprintf("./%s/%s", bundle.Path, metaData.Info.Name)}
		bundle.Files = []*BundleFile{bundleFile}
		totalLength = metaData.Info.Length
	}
	bundle.Length = totalLength
	if debugopts.BUNDLE_DEBUG {
		fmt.Println("Got files...")
		for _, file := range bundle.Files {
			fmt.Printf("Path: %s, Length: %d\n", file.Path, file.Length)
		}
	}
	if metaData.Info.PieceLength == 0 {
		return nil, errors.New("no piece length")
	}
	// Pieces
	if len(metaData.Info.Pieces) == 0 {
		return nil, errors.New("no pieces")
	}
	curByte := int64(0)
	bundle.PieceLength = metaData.Info.PieceLength
	for _, pieceHash := range metaData.Info.Pieces {
		curPieceLength := bundle.PieceLength
		if curByte+bundle.PieceLength > totalLength {
			curPieceLength = totalLength - curByte
		}
		curPiece := NewPiece(curPieceLength, curByte, pieceHash)
		bundle.Pieces = append(bundle.Pieces, curPiece)
		curByte += curPieceLength
	}
	bundle.NumPieces = int64(len(bundle.Pieces))
	if debugopts.BUNDLE_DEBUG {
		fmt.Printf("Got pieces (%d)\n", bundle.NumPieces)
		//		for i, piece := range bundle.Pieces {
		//fmt.Printf("Piece %d hash: %v\n", i, piece.hash)
		//}
	}

	// bitfield

	bundle.Bitfield = bitfield.New(bundle.NumPieces)

	// Write files

	for _, bundleFile := range bundle.Files {
		if debugopts.BUNDLE_DEBUG {
			fmt.Printf("Creating file: %s (%d Bytes)\n", bundleFile.Path, bundleFile.Length)
		}
		if !checkFile(bundleFile.Path, bundleFile.Length) {
			file, err := os.OpenFile(bundleFile.Path, os.O_WRONLY|os.O_CREATE, 0644)
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

func (bundle *Bundle) DeleteFiles() error {
	if bundle.MultiFile {
		for _, fileInfo := range bundle.Files {
			err := os.Remove(fileInfo.Path)
			if err != nil {
				fmt.Printf("Error deleting file (%s): %v\n", fileInfo.Path, err)
			}
		}
	}
	return nil
}

func (bundle *Bundle) WriteBlock(pieceIndex int64, beginOffset int64, block []byte) error { // beginOffset will always match a block in pieces
	bundle.mux.Lock()
	defer bundle.mux.Unlock()

	err := bundle.Pieces[pieceIndex].WriteBlock(beginOffset, block)
	if err != nil {
		return err
	}
	if bundle.Pieces[pieceIndex].Complete {
		if debugopts.BUNDLE_DEBUG {
			fmt.Printf("Pieces[%d] complete... writing piece...\n", pieceIndex)
		}
		bundle.Bitfield.SetBit(pieceIndex)
		err := bundle.writePiece(pieceIndex)
		if err != nil {
			return err
		}
	}
	if bundle.Bitfield.Complete {
		fmt.Println("Bundle download complete!!!!!")
		bundle.Complete = true
	}
	if debugopts.BUNDLE_DEBUG {
		fmt.Printf("Saved block: index: %d, offset: %d, length: %d\n", pieceIndex, beginOffset, len(block))
		fmt.Printf("Current state: ")
		bundle.Bitfield.Print()
	}
	return err
}

func (bundle *Bundle) writePiece(pieceIndex int64) error { // Private and only called after mux lock so
	piece := bundle.Pieces[pieceIndex]
	bytes := piece.GetBytes()
	if !bundle.MultiFile {
		file, err := os.OpenFile(bundle.Name, os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		_, err = file.WriteAt(bytes, piece.ByteOffset)
		if err != nil {
			panic(err)
		}
	} else { // Else multifile
		pieceByteWriteStartOffset := int64(0)
		for _, bundleFile := range bundle.Files {
			if bundleFile.ByteStart > pieceByteWriteStartOffset+piece.ByteOffset ||
				bundleFile.ByteStart+bundleFile.Length < pieceByteWriteStartOffset+piece.ByteOffset { // if bytes to write do not start in this file
				continue
			}
			pieceByteWriteEndOffset := piece.length
			if pieceByteWriteEndOffset+piece.ByteOffset > bundleFile.ByteStart+bundleFile.Length { // if the piece ends after the end of the file
				pieceByteWriteEndOffset = bundleFile.ByteStart + bundleFile.Length - piece.ByteOffset // set pieceByteWriteEndOffset to the last byte in the file
			}
			bytesToWrite := bytes[pieceByteWriteStartOffset:pieceByteWriteEndOffset] // Reslice to correct bytes to write
			file, err := os.OpenFile(bundleFile.Path, os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			fileWriteOffset := pieceByteWriteStartOffset + piece.ByteOffset - bundleFile.ByteStart
			_, err = file.WriteAt(bytesToWrite, fileWriteOffset)
			if err != nil {
				return err
			}
			// writePiece
			if pieceByteWriteEndOffset == piece.ByteOffset+piece.length { // if last byte in the piece is written
				break
			}
			pieceByteWriteStartOffset = pieceByteWriteEndOffset // start next write at current end
		} // end iterating through files
	}
	return nil
}

func (bundle *Bundle) loadPiece(pieceIndex int64) error {
	pieceBytes, err := bundle.readPiece(pieceIndex)
	if err != nil {
		return err
	}
	bundle.pieceCache.Put(pieceIndex, pieceBytes)
	return nil
}

func (bundle *Bundle) readPiece(pieceIndex int64) ([]byte, error) {
	pieceBytes := []byte{}
	bytesLeft := bundle.Pieces[pieceIndex].length
	byteOffset := bundle.Pieces[pieceIndex].ByteOffset
	for _, bundleFile := range bundle.Files {
		if bundleFile.ByteStart <= byteOffset && bundleFile.ByteStart+bundleFile.Length <= byteOffset {
			fileByteOffset := byteOffset - bundleFile.ByteStart
			readLen := bytesLeft
			if fileByteOffset+bytesLeft > bundleFile.Length {
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
			pieceBytes = append(pieceBytes, buf...)
			bytesLeft -= readLen
			byteOffset += readLen
			if bytesLeft == 0 {
				break
			}
		}
	}
	return pieceBytes, nil
}

func (bundle *Bundle) GetBlock(bi *BlockInfo) ([]byte, error) {
	bundle.mux.Lock()
	defer bundle.mux.Unlock()

	if !bundle.pieceCache.Contains(bi.PieceIndex) {
		bundle.loadPiece(bi.PieceIndex)
	}
	pieceBytes := bundle.pieceCache.Get(bi.PieceIndex)

	if bi.Length+bi.BeginOffset > bundle.Pieces[bi.PieceIndex].length {
		return nil, errors.New("block overflows piece bounds")
	}

	return pieceBytes[bi.BeginOffset : bi.Length+bi.BeginOffset], nil
}

func (bundle *Bundle) CancelBlock(bi *BlockInfo) error {
	bundle.mux.Lock()
	defer bundle.mux.Unlock()

	err := bundle.Pieces[bi.PieceIndex].CancelBlock(bi.BeginOffset)
	if err != nil {
		return err
	}
	return nil
}

func (bundle *Bundle) NextNBlocks(numBlocks int) []*BlockInfo {
	retBlocks := make([]*BlockInfo, 0)
	for pIndex, piece := range bundle.Pieces {
		if piece.Complete {
			continue
		}
		blockByteOffset := int64(0)
		for bIndex, block := range piece.Blocks {
			if !block.Fetching && !block.Written {
				bundle.Pieces[pIndex].Blocks[bIndex].Fetching = true
				retBlocks = append(retBlocks, &BlockInfo{
					PieceIndex:  int64(pIndex),
					BeginOffset: blockByteOffset,
					Length:      block.Length,
				})
			}
			blockByteOffset += block.Length
			if len(retBlocks) == numBlocks {
				break
			}
		}
		if len(retBlocks) == numBlocks {
			break
		}
	}
	return retBlocks
}

func (bundle *Bundle) BytesLeft() int64 {
	return (bundle.Bitfield.Len() - bundle.Bitfield.NumSet) * bundle.PieceLength
}

func (bundle *Bundle) PrintStatus() {
	fmt.Printf("Bundle status: \n")
	fmt.Printf("InfoHash: %x\nHave: %d\nTotal Pieces: %d\n", bundle.InfoHash, bundle.Bitfield.NumSet, bundle.NumPieces)
}

func (bundle *Bundle) Recheck() {
	bundle.Bitfield.ResetAll()
	for pieceIndex := int64(0); pieceIndex < int64(len(bundle.Pieces)); pieceIndex++ {
		pieceBytes, err := bundle.readPiece(pieceIndex)
		if err != nil {
			fmt.Println(err)
			return
		}
		hasher := sha1.New()
		_, err = hasher.Write(pieceBytes)
		if err != nil {
			fmt.Println(err)
			return
		}
		pieceHash := hasher.Sum(nil)
		if bytes.Equal(pieceHash, bundle.Pieces[pieceIndex].hash) {
			bundle.Bitfield.SetBit(pieceIndex)
		}
	}
	fmt.Printf("Recheck complete bitfield: ")
	bundle.Bitfield.Print()
}
