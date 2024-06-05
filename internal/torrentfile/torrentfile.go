package torrentfile

import (
	"errors"
	"fmt"
	"os"
)

type FilePath = []string

type FileInfo struct {
	Length uint64
	Path   FilePath
}

type TorrentInfo struct {
	Name        string
	PieceLength uint32 //bytes
	Pieces      [][]byte
	MultiFile   bool
	Files       []FileInfo
	Length      uint64
}

type TorrentFile struct {
	Data		 []byte
	Announce     string
	AnnounceList [][]string
	Info         TorrentInfo
	CreationDate string
	Comment      string
	CreatedBy    string
}

type TokenType int

const (
	String TokenType = iota
	Integer
	List
	Dictionary
	EndOfList
	EndOfDictionary
)

type Token struct {
	Type TokenType
	Value []byte
}

func (tf *TorrentFile) Tokenize() (*[]Token, error) {
	curContainers := []Token{}
	tokens := []Token{}
	i := 0
	for i < len(tf.Data) {
		if tf.Data[i] == 'd' {
			tokens = append(tokens, Token{Type: Dictionary, Value: []byte{}})
			curContainers = append(curContainers, Token{Type: Dictionary, Value: []byte{}})
		} else if tf.Data[i] == 'l' {
			tokens = append(tokens, Token{Type: List, Value: []byte{}})
			curContainers = append(curContainers, Token{Type: List, Value: []byte{}})
		} else if tf.Data[i] == 'i' {
			i++
			newInt := Token{Type: Integer, Value: []byte{}}
			for (tf.Data[i] != 'e') {
				newInt.Value = append(newInt.Value, tf.Data[i])
				i++
			}
			tokens = append(tokens, newInt)
		} else if tf.Data[i] == 'e' {
			if len(curContainers) > 0 && curContainers[len(curContainers)-1].Type == Dictionary {
				tokens = append(tokens, Token{Type: EndOfDictionary, Value: []byte{}})
				curContainers = curContainers[:len(curContainers) - 1]
			} else if len(curContainers) > 0 && curContainers[len(curContainers)-1].Type == List {
				tokens = append(tokens, Token{Type: EndOfList, Value: []byte{}})
				curContainers = curContainers[:len(curContainers) - 1]
			} 
		} else { //byte string
			newString := Token{Type: String, Value: []byte{}}
			if tf.Data[i] < '0' || tf.Data[i] > '9' {
				return nil, errors.New("invalid byte in data")
			}
			stringLength := 0
			for i < len(tf.Data) && tf.Data[i] != ':' {
				stringLength = stringLength*10
				stringLength += int(tf.Data[i] - '0')
				i++
			}
			for j := 0; j < stringLength; j++ {
				i++
				newString.Value = append(newString.Value, tf.Data[i])
			}
			tokens = append(tokens, newString)
		}
		i++
	}
	return &tokens, nil
}


func PrintTokens(tokens *[]Token) {
	for i := range *tokens {
		curToken := (*tokens)[i]
		var typeString string
		switch curToken.Type {
		case String:
			typeString = "String"
		case Integer:
			typeString = "Integer"
		case List:
			typeString = "List"
		case Dictionary:
			typeString = "Dictionary"
		case EndOfDictionary:
			typeString = "EndOfDictionary"
		case EndOfList:
			typeString = "EndOfList"
		}
		if len(curToken.Value) > 0 {
			fmt.Printf("%s (", typeString)
			for _, val := range curToken.Value {
				fmt.Printf("%c, ", val)
			}
			fmt.Printf(")\n")
		} else {
			fmt.Printf("%s\n", typeString)
		}
	}
}

func (*TorrentFile) parseList() {
	
}

func (*TorrentFile) parseDict() {

}

func ParseFile(path string) (*TorrentFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = Tokenize(&data)
	
	if err != nil {
		return nil, err
	}
	return nil, nil
}