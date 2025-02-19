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
	"path/filepath"
	"sync"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/debugopts"
	"github.com/GeminiZA/GoTorrentServer/internal/logger"
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
	logger      *logger.Logger
}

// Create files, if exist recheck
func Create(metaData *torrentfile.TorrentFile, bundlePath string) (*Bundle, error) {
	bundle := Bundle{
		Name:        metaData.Info.Name,
		PieceLength: metaData.Info.PieceLength,
		Complete:    false,
		Path:        bundlePath,
		pieceCache:  NewPieceCache(1280), // 1280 * 16 KiB: 20 MiB
		InfoHash:    metaData.InfoHash,
		logger:      logger.New(logger.WARN, "Bundle"),
	}
	// Check existence
	// Create bundle Path
	if _, err := os.Stat(bundle.Path); err != nil {
		err := os.MkdirAll(bundle.Path, 0755)
		if err != nil {
			return nil, err
		}
	}
	// Files
	if metaData.Info.Length == 0 {
		bundle.MultiFile = true
		// if multifile append torrent name to path
		if _, err := os.Stat(filepath.Join(bundlePath, metaData.Info.Name)); err != nil {
			err := os.MkdirAll(filepath.Join(bundlePath, metaData.Info.Name), 0755)
			if err != nil {
				return nil, err
			}
		}
		curByte := int64(0)
		bundle.Files = make([]*BundleFile, 0)
		for _, file := range metaData.Info.Files {
			curBundleFile := BundleFile{
				Path:      filepath.Join(bundlePath, metaData.Info.Name),
				Length:    file.Length,
				ByteStart: curByte,
			}
			curByte += file.Length
			for index, pathPiece := range file.Path {
				if index != len(file.Path)-1 {
					curBundleFile.Path = filepath.Join(curBundleFile.Path, pathPiece)
				}
			}
			_, err := os.Stat(curBundleFile.Path)
			if err != nil {
				if os.IsNotExist(err) {
					err := os.MkdirAll(curBundleFile.Path, 0755)
					if err != nil {
						return nil, err
					}
				} else {
					return nil, err
				}
			}
			curBundleFile.Path = filepath.Join(curBundleFile.Path, file.Path[len(file.Path)-1])
			bundle.Files = append(bundle.Files, &curBundleFile)
		}
		bundle.Length = curByte

	} else {
		bundle.MultiFile = false
		bundle.Files = []*BundleFile{{
			Path:      filepath.Join(bundle.Path, metaData.Info.Name),
			Length:    metaData.Info.Length,
			ByteStart: 0,
		}}
		bundle.Length = metaData.Info.Length
	}

	curByte := int64(0)
	for _, pieceHash := range metaData.Info.Pieces {
		curPieceLength := bundle.PieceLength
		if curByte+bundle.PieceLength > bundle.Length {
			curPieceLength = bundle.Length - curByte
		}
		curPiece := NewPiece(curPieceLength, curByte, pieceHash)
		bundle.Pieces = append(bundle.Pieces, curPiece)
		curByte += curPieceLength
	}
	bundle.NumPieces = int64(len(bundle.Pieces))

	bundle.Bitfield = bitfield.New(bundle.NumPieces)

	foundFiles, err := bundle.writeFiles()
	if err != nil {
		deleteErr := bundle.DeleteFiles()
		if deleteErr != nil {
			bundle.logger.Error(fmt.Sprintf("Error deleting file: %v", deleteErr))
		}
		return nil, err
	}
	if foundFiles {
		err := bundle.Recheck()
		if err != nil {
			bundle.logger.Debug(fmt.Sprintf("Recheck failed: %v", err))
		}
	}
	return &bundle, nil
}

func (bundle *Bundle) writeFiles() (bool, error) {
	foundFiles := false
	for _, bundleFile := range bundle.Files {
		if info, err := os.Stat(bundleFile.Path); err != nil || info.Size() != bundleFile.Length {
			if os.IsNotExist(err) || info.Size() != bundleFile.Length {
				// Write file
				bundle.logger.Debug(fmt.Sprintf("Creating file: %s (%d Bytes)", bundleFile.Path, bundleFile.Length))
				file, err := os.OpenFile(bundleFile.Path, os.O_WRONLY|os.O_CREATE, 0755)
				if err != nil {
					return false, err
				}
				defer file.Close()
				curByte := int64(0)
				emptyMB := make([]byte, 1024*1024)
				for i := range emptyMB {
					emptyMB[i] = 0
				}
				for curByte+1024*1024 < bundleFile.Length {
					_, err = file.Write(emptyMB)
					if err != nil {
						return false, err
					}
					curByte += 1024 * 1024
				}
				emptyKB := make([]byte, 1024)
				for i := range emptyKB {
					emptyKB[i] = 0
				}
				for curByte+1024 < bundleFile.Length {
					_, err = file.Write(emptyKB)
					if err != nil {
						return false, err
					}
					curByte += 1024
				}
				for curByte+1 < bundleFile.Length {
					_, err = file.Write([]byte{0})
					if err != nil {
						return false, err
					}
					curByte += 1024
				}
			} else {
				return false, err
			}
		} else {
			foundFiles = true
		}
	}

	return foundFiles, nil
}

// Load files; fail if doesn't exist or lengths are wrong
func Load(metaData *torrentfile.TorrentFile, bf *bitfield.Bitfield, bundlePath string) (*Bundle, error) {
	bundle := Bundle{
		Name:        metaData.Info.Name,
		PieceLength: metaData.Info.PieceLength,
		Complete:    false,
		Path:        bundlePath,
		pieceCache:  NewPieceCache(1280), // 1280 * 16 KiB: 20 MiB
		InfoHash:    metaData.InfoHash,
		logger:      logger.New(logger.ERROR, "Bundle"),
	}

	if _, err := os.Stat(bundle.Path); err != nil {
		return nil, err
	}
	// Files
	if metaData.Info.Length == 0 {
		bundle.MultiFile = true
		bundle.Path = filepath.Join(bundlePath, metaData.Info.Name)
		curByte := int64(0)
		bundle.Files = make([]*BundleFile, 0)
		for _, file := range metaData.Info.Files {
			curBundleFile := BundleFile{
				Path:      bundle.Path,
				Length:    file.Length,
				ByteStart: curByte,
			}
			curByte += file.Length
			for _, pathPiece := range file.Path {
				curBundleFile.Path = filepath.Join(curBundleFile.Path, pathPiece)
			}
			_, err := os.Stat(curBundleFile.Path)
			if err != nil {
				return nil, err
			}
			bundle.Files = append(bundle.Files, &curBundleFile)
		}
		bundle.Length = curByte
	} else {
		bundle.MultiFile = false
		bundle.Files = []*BundleFile{{
			Path:      filepath.Join(bundle.Path, metaData.Info.Name),
			Length:    metaData.Info.Length,
			ByteStart: 0,
		}}
		if _, err := os.Stat(bundle.Files[0].Path); err != nil {
			return nil, err
		}
		bundle.Length = metaData.Info.Length
	}

	curByte := int64(0)
	for _, pieceHash := range metaData.Info.Pieces {
		curPieceLength := bundle.PieceLength
		if curByte+bundle.PieceLength > bundle.Length {
			curPieceLength = bundle.Length - curByte
		}
		curPiece := NewPiece(curPieceLength, curByte, pieceHash)
		bundle.Pieces = append(bundle.Pieces, curPiece)
		curByte += curPieceLength
	}
	bundle.NumPieces = int64(len(bundle.Pieces))

	bundle.Bitfield = bf

	return &bundle, nil
}

func (bundle *Bundle) DeleteFiles() error {
	err := os.RemoveAll(bundle.Path)
	if err != nil {
		return err
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
		bundle.logger.Debug(fmt.Sprintf("Pieces[%d] complete... writing piece...\n", pieceIndex))
		bundle.Bitfield.SetBit(pieceIndex)
		err := bundle.writePiece(pieceIndex)
		if err != nil {
			return err
		}
	}
	if bundle.Bitfield.Complete {
		bundle.Complete = true
	}
	msg := fmt.Sprintf("Saved block: index: %d, offset: %d, length: %d\n", pieceIndex, beginOffset, len(block))
	msg += "Current state: "
	msg += bundle.Bitfield.Print()
	return err
}

func (bundle *Bundle) writePiece(pieceIndex int64) error { // Private and only called after mux lock so
	timeStartWrite := time.Now()
	piece := bundle.Pieces[pieceIndex]
	bytes := piece.GetBytes()
	if !bundle.MultiFile {
		bundleFile := bundle.Files[0]
		file, err := os.OpenFile(bundleFile.Path, os.O_WRONLY, 0755)
		if err != nil {
			return err
		}
		_, err = file.WriteAt(bytes, piece.ByteOffset)
		if err != nil {
			return err
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
			file, err := os.OpenFile(bundleFile.Path, os.O_WRONLY, 0755)
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
	timeWrite := time.Now().Sub(timeStartWrite)
	bundle.logger.Debug(fmt.Sprintf("Wrote piece %d (size: %d KiB); Time: %s", pieceIndex, bundle.Pieces[pieceIndex].length, timeWrite))
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
	piece := bundle.Pieces[pieceIndex]
	pieceBytes := make([]byte, 0, piece.length)

	if !bundle.MultiFile {
		file, err := os.OpenFile(bundle.Name, os.O_RDONLY, 0755)
		if err != nil {
			return nil, err
		}
		_, err = file.ReadAt(pieceBytes, piece.ByteOffset)
		if err != nil {
			return nil, err
		}
	} else { // Else multifile
		pieceByteReadStartOffset := int64(0)
		for _, bundleFile := range bundle.Files {
			if bundleFile.ByteStart > pieceByteReadStartOffset+piece.ByteOffset ||
				bundleFile.ByteStart+bundleFile.Length < pieceByteReadStartOffset+piece.ByteOffset { // if bytes to write do not start in this file
				continue
			}
			pieceByteReadEndOffset := piece.length
			if pieceByteReadEndOffset+piece.ByteOffset > bundleFile.ByteStart+bundleFile.Length { // if the piece ends after the end of the file
				pieceByteReadEndOffset = bundleFile.ByteStart + bundleFile.Length - piece.ByteOffset // set pieceByteReadEndOffset to the last byte in the file
			}
			lenToRead := pieceByteReadEndOffset - pieceByteReadStartOffset
			bytesRead := make([]byte, lenToRead)
			file, err := os.OpenFile(bundleFile.Path, os.O_RDONLY, 0755)
			if err != nil {
				return nil, err
			}
			fileReadOffset := pieceByteReadStartOffset + piece.ByteOffset - bundleFile.ByteStart
			_, err = file.ReadAt(bytesRead, fileReadOffset)
			if err != nil {
				return nil, err
			}
			pieceBytes = append(pieceBytes, bytesRead...)

			// readPiece
			if pieceByteReadEndOffset == piece.ByteOffset+piece.length { // if last byte in the piece is written
				break
			}
			pieceByteReadStartOffset = pieceByteReadEndOffset // start next write at current end
		} // end iterating through files
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

func (bundle *Bundle) PrintStatus() string {
	return fmt.Sprintf("Bundle status: \nInfoHash: %x\nHave: %d\nTotal Pieces: %d\n", bundle.InfoHash, bundle.Bitfield.NumSet, bundle.NumPieces)
}

func (bundle *Bundle) Recheck() error {
	if debugopts.BUNDLE_DEBUG {
		bundle.logger.Debug(fmt.Sprintln("Rechecking bundle..."))
	}
	bundle.Bitfield.ResetAll()
	for pieceIndex := int64(0); pieceIndex < int64(len(bundle.Pieces)); pieceIndex++ {
		pieceBytes, err := bundle.readPiece(pieceIndex)
		if err != nil {
			bundle.logger.Error(fmt.Sprintf("Error reading piece: %v\n", err))
			return err
		}
		// fmt.Printf("Read bytes for piece(%d): %x\n", pieceIndex, pieceBytes)
		hasher := sha1.New()
		_, err = hasher.Write(pieceBytes)
		if err != nil {
			return err
		}
		pieceHash := hasher.Sum(nil)
		// fmt.Printf("Rechecking Piece(%d): Hash: %x, CheckedHash: %x\n", pieceIndex, bundle.Pieces[pieceIndex].hash, pieceHash)
		if bytes.Equal(pieceHash, bundle.Pieces[pieceIndex].hash) {
			bundle.Bitfield.SetBit(pieceIndex)
		}
	}
	bundle.logger.Debug(fmt.Sprintf("Recheck complete bitfield: %s", bundle.Bitfield.Print()))
	bundle.Complete = bundle.Bitfield.Complete
	return nil
}
