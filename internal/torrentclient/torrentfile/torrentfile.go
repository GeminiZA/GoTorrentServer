package torrentfile

import (
	"errors"
	"fmt"
	"os"

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
	MultiFile	bool
}

type TorrentFile struct {
	InfoHash 	 []byte
	Announce     string
	AnnounceList [][]string
	CreationDate int64
	Comment      string
	CreatedBy    string
	Encoding     string
	Info         TorrentInfo
}

func (tf *TorrentFile) Bencode() (string, error) {
	dict := make(map[string]interface{})
	dict["announce"] = tf.Announce
	if len(tf.AnnounceList) > 0 {
		dict["announce list"] = tf.AnnounceList
	}
	dict["creation date"] = tf.CreationDate
	dict["comment"] = tf.Comment
	dict["created by"] = tf.CreatedBy
	dict["encoding"] = tf.Encoding
	infoDict := make(map[string]interface{})
	dict["info"] = infoDict 
	infoDict["name"] = tf.Info.Name
	infoDict["piece length"] = tf.Info.PieceLength
	piecesString := ""
	for _, piece := range tf.Info.Pieces {
		piecesString += string(piece)
	}
	infoDict["pieces"] = piecesString
	infoDict["private"] = tf.Info.Private
	infoDict["length"] = tf.Info.Length
	if len(tf.Info.Files) > 0 {
		filesList := make([]interface{}, 0)
		for _, file := range tf.Info.Files {
			filesList = append(filesList, make(map[string]interface{}))
			if fileDict, ok := filesList[len(filesList)-1].(map[string]interface{}); ok {
				fileDict["length"] = file.Length
				fileDict["path"] = file.Path
			}
		}
		infoDict["files"] = filesList
	}
	bencodedFile, err := bencode.BEncode(dict)
	
	return bencodedFile, err
}

func ParseFileString(data *[]byte) (*TorrentFile, error) {
	var tf TorrentFile
	var err error

	tf.InfoHash, err = bencode.GetInfoHash(data)
	//fmt.Printf("Got info hash: %s\n", string(tf.InfoHash))

	if err != nil {
		panic(err)
	}

	tokens, err := bencode.Tokenize(data)

	if err != nil {
		return nil, err
	}
	//fmt.Printf("Succesfully tokenized file\n")

	dict, err := bencode.ParseDict(tokens)
	if err != nil {
		return nil, err
	}
	//fmt.Printf("Successfully parsed dict\n")
	
	var ok bool
	tf.Announce, ok = dict["announce"].(string)
	if !ok {
		return nil, errors.New("no announce in file")
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
		return nil, errors.New("no creation date")
	}
	tf.Comment, _ = dict["comment"].(string)
	tf.CreatedBy, ok = dict["created by"].(string)
	if !ok {
		return nil, errors.New("no created by")
	}
	tf.Encoding, _ = dict["encoding"].(string)
	infoDict, ok := dict["info"].(map[string]interface{})
	if !ok {
		return nil, errors.New("no info")
	}
	tf.Info.Name, ok = infoDict["name"].(string)
	if !ok {
		return nil, errors.New("no name")
	}
	tf.Info.PieceLength, ok = infoDict["piece length"].(int64)
	if !ok {
		return nil, errors.New("no piece length")
	}
	//pieces
	piecesString, ok := infoDict["pieces"].(string)
	if !ok {
		return nil, errors.New("no pieces")
	}
	if len(piecesString) % 20 != 0 {
		return nil, errors.New("pieces length not divisible by 20")
	}
	fmt.Printf("Pieces string length: %d\n", len(piecesString))
	for i := 0; i < len(piecesString); i += 20 {
		tf.Info.Pieces = append(tf.Info.Pieces, []byte(piecesString[i:i+20]))
	}
	privateBool, ok := infoDict["private"].(int64)
	if !ok {
		return nil, errors.New("no private")
	}
	tf.Info.Private = privateBool != 0
	length, ok := infoDict["length"].(int64)
	if ok {
		tf.Info.Length = length
		tf.Info.MultiFile = false
	} else {
		files, ok := infoDict["files"].([]interface{})
		if !ok {
			return nil, errors.New("no files or length")
		}
		tf.Info.MultiFile = true
		for _, file := range files {
			fileItem, ok := file.(map[string]interface{})
			if !ok {
				return nil, errors.New("invalid file item")
			}
			length, ok := fileItem["length"].(int64)
			if !ok {
				return nil, errors.New("no length in file")
			}
			var pathList []string
			pathListInterface, ok := fileItem["path"].([]interface{})
			if !ok {
				return nil, errors.New("no path in file")
			}
			for _, path := range pathListInterface {
				pathString, ok := path.(string)
				if !ok {
					return nil, errors.New("invalid path list in file")
				}
				pathList = append(pathList, pathString)
			}
			tf.Info.Files = append(tf.Info.Files, FileInfo{Path: pathList, Length: length})
		}
	}
	return &tf, nil
}

func ParseFile(path string) (*TorrentFile, error) {
	fmt.Printf("Parsing file with path: %s\n", path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	tf, err := ParseFileString(&data)

	return tf, err
}