package torrentfile

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/logger"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bencode"
)

type FileInfo struct {
	Path   []string
	Length int64
}

type TorrentInfo struct {
	Name        string
	PieceLength int64
	Pieces      [][]byte
	Private     bool
	Length      int64
	Files       []FileInfo
	MultiFile   bool
}

type TorrentFile struct {
	dict         map[string]interface{}
	InfoHash     []byte
	Announce     string
	AnnounceList [][]string
	CreationDate int64
	Comment      string
	CreatedBy    string
	Encoding     string
	Info         TorrentInfo
	logger       *logger.Logger
}

func (tf *TorrentFile) Bencode() (string, error) {
	return bencode.BEncode(tf.dict)
}

func New() *TorrentFile {
	return &TorrentFile{
		logger: logger.New(logger.DEBUG, "TorrentFile"),
	}
}

func (tf *TorrentFile) ParseFileString(data *[]byte) error {
	var err error

	timeStart := time.Now()

	tf.InfoHash, err = bencode.GetInfoHash(data)
	if err != nil {
		panic(err)
	}
	tf.logger.Debug(fmt.Sprintf("Got info hash: %s\n", string(tf.InfoHash)))

	tokens, err := bencode.Tokenize(data)
	if err != nil {
		return err
	}
	tf.logger.Debug("Succesfully tokenized file\n")

	dict, err := bencode.ParseDict(tokens)
	if err != nil {
		return err
	}
	tf.logger.Debug("Successfully parsed dict\n")

	tf.dict = dict

	var ok bool
	tf.Announce, ok = dict["announce"].(string)
	if !ok {
		return errors.New("no announce in file")
	}
	rawAnnounceList, ok := dict["announce-list"].([]interface{})
	if ok {
		for i := range rawAnnounceList {
			tf.AnnounceList = append(tf.AnnounceList, make([]string, 0))
			list, ok := rawAnnounceList[i].([]interface{})
			if ok {
				for j := range list {
					item, ok := list[j].(string)
					if ok {
						tf.AnnounceList[len(tf.AnnounceList)-1] = append(tf.AnnounceList[len(tf.AnnounceList)-1], item)
					}
				}
			}
		}
	}
	tf.CreationDate, ok = dict["creation date"].(int64)
	if !ok {
		return errors.New("no creation date")
	}
	tf.Comment, _ = dict["comment"].(string)
	tf.CreatedBy, ok = dict["created by"].(string)
	if !ok {
		return errors.New("no created by")
	}
	tf.Encoding, _ = dict["encoding"].(string)
	infoDict, ok := dict["info"].(map[string]interface{})
	if !ok {
		return errors.New("no info")
	}
	tf.Info.Name, ok = infoDict["name"].(string)
	if !ok {
		return errors.New("no name")
	}
	tf.Info.PieceLength, ok = infoDict["piece length"].(int64)
	if !ok {
		return errors.New("no piece length")
	}
	// pieces
	piecesString, ok := infoDict["pieces"].(string)
	if !ok {
		return errors.New("no pieces")
	}
	if len(piecesString)%20 != 0 {
		return errors.New("pieces length not divisible by 20")
	}
	tf.logger.Debug(fmt.Sprintf("Pieces string length: %d\n", len(piecesString)))
	for i := 0; i < len(piecesString); i += 20 {
		tf.Info.Pieces = append(tf.Info.Pieces, []byte(piecesString[i:i+20]))
	}
	privateBool, ok := infoDict["private"].(int64)
	if ok {
		tf.Info.Private = privateBool != 0
	}
	length, ok := infoDict["length"].(int64)
	if ok {
		tf.Info.Length = length
		tf.Info.MultiFile = false
	} else {
		tf.Info.Length = 0
		files, ok := infoDict["files"].([]interface{})
		if !ok {
			return errors.New("no files or length")
		}
		tf.Info.MultiFile = true
		for _, file := range files {
			fileItem, ok := file.(map[string]interface{})
			if !ok {
				return errors.New("invalid file item")
			}
			length, ok := fileItem["length"].(int64)
			if !ok {
				return errors.New("no length in file")
			}
			var pathList []string
			pathListInterface, ok := fileItem["path"].([]interface{})
			if !ok {
				return errors.New("no path in file")
			}
			for _, path := range pathListInterface {
				pathString, ok := path.(string)
				if !ok {
					return errors.New("invalid path list in file")
				}
				pathList = append(pathList, pathString)
			}
			tf.Info.Files = append(tf.Info.Files, FileInfo{Path: pathList, Length: length})
		}
	}
	tf.logger.Debug(fmt.Sprintf("Torrentfile parsed successfully; Time: %s", time.Since(timeStart)))
	return nil
}

func (tf *TorrentFile) ParseFile(path string) error {
	tf.logger.Debug(fmt.Sprintf("Parsing file with path: %s\n", path))
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	err = tf.ParseFileString(&data)

	return err
}
