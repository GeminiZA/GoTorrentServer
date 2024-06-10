package bencode

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"strconv"
)

type TokenType int

const (
	STRING TokenType = iota
	INTEGER
	LIST
	DICTIONARY
	END_OF_LIST
	END_OF_DICTIONARY
)

type Token struct {
	Type TokenType
	Value []byte
}


func Tokenize(data *[]byte) ([]Token, error) {
	//fmt.Printf("Tokenizing file\n")
	curContainers := []Token{}
	tokens := []Token{}
	i := 0
	for i < len((*data)) {
		if (*data)[i] == 'd' {
			tokens = append(tokens, Token{Type: DICTIONARY, Value: []byte{}})
			curContainers = append(curContainers, Token{Type: DICTIONARY, Value: []byte{}})
		} else if (*data)[i] == 'l' {
			tokens = append(tokens, Token{Type: LIST, Value: []byte{}})
			curContainers = append(curContainers, Token{Type: LIST, Value: []byte{}})
		} else if (*data)[i] == 'i' {
			i++
			newInt := Token{Type: INTEGER, Value: []byte{}}
			for ((*data)[i] != 'e') {
				newInt.Value = append(newInt.Value, (*data)[i])
				i++
			}
			tokens = append(tokens, newInt)
		} else if (*data)[i] == 'e' {
			if len(curContainers) > 0 && curContainers[len(curContainers)-1].Type == DICTIONARY {
				tokens = append(tokens, Token{Type: END_OF_DICTIONARY, Value: []byte{}})
				curContainers = curContainers[:len(curContainers) - 1]
			} else if len(curContainers) > 0 && curContainers[len(curContainers)-1].Type == LIST {
				tokens = append(tokens, Token{Type: END_OF_LIST, Value: []byte{}})
				curContainers = curContainers[:len(curContainers) - 1]
			} 
		} else { //byte string
			newString := Token{Type: STRING, Value: []byte{}}
			if (*data)[i] < '0' || (*data)[i] > '9' {
				return nil, errors.New("invalid byte in data")
			}
			stringLength := 0
			for i < len((*data)) && (*data)[i] != ':' {
				stringLength = stringLength*10
				stringLength += int((*data)[i] - '0')
				i++
			}
			for j := 0; j < stringLength; j++ {
				i++
				newString.Value = append(newString.Value, (*data)[i])
			}
			tokens = append(tokens, newString)
		}
		i++
	}
	return tokens, nil
}

func PrintToken(token Token) {
	typeString := ""
	switch token.Type {
	case STRING:
		typeString = "String"
	case INTEGER:
		typeString = "INTEGER"
	case LIST:
		typeString = "LIST"
	case DICTIONARY:
		typeString = "DICTIONARY"
	case END_OF_DICTIONARY:
		typeString = "END_OF_DICTIONARY"
	case END_OF_LIST:
		typeString = "END_OF_LIST"
	}
	valString := string(token.Value)
	if token.Type == STRING || token.Type == INTEGER {
		fmt.Printf("{%s: %s}", typeString, valString)
	} else {
		fmt.Printf("{%s}", typeString)
	}
}


func PrintTokens(tokens []Token) {
	for i := range tokens {
		curToken := tokens[i]
		var typeString string
		switch curToken.Type {
		case STRING:
			typeString = "String"
		case INTEGER:
			typeString = "INTEGER"
		case LIST:
			typeString = "LIST"
		case DICTIONARY:
			typeString = "DICTIONARY"
		case END_OF_DICTIONARY:
			typeString = "END_OF_DICTIONARY"
		case END_OF_LIST:
			typeString = "END_OF_LIST"
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

func ParseString(token Token) (string, error) {
	return string(token.Value), nil
}

func ParseInteger(token Token) (int64, error) {
	retInt, err := strconv.ParseInt(string(token.Value), 10, 64)
	if err != nil {
		return 0, err
	}
	return retInt, nil
}

func ParseList(tokens []Token) ([]interface{}, error) {
	var list []interface{}
	if tokens[0].Type != LIST || tokens[len(tokens) - 1].Type != END_OF_LIST {
		return nil, errors.New("invalid list form")
	}
	i := 1
	for i < len(tokens) - 1 {
		switch tokens[i].Type {
		case STRING:
			item, err := ParseString(tokens[i])
			if err != nil {
				return nil, err
			}
			list = append(list, item)
		case INTEGER:
			item, err := ParseInteger(tokens[i])
			if err != nil {
				return nil, err
			}
			list = append(list, item)
		case LIST:
			listStart := i
			listCount := 1
			i++
			for i < len(tokens) - 1 && listCount > 0 {
				switch tokens[i].Type {
				case LIST:
					listCount++
				case END_OF_LIST:
					listCount--
				}
				i++
			}
			i--
			item, err := ParseList(tokens[listStart:i + 1])
			if err != nil {
				return nil, err
			}
			list = append(list, item)
		case DICTIONARY:
			dictStart := i
			dictCount := 1
			i++
			for i < len(tokens) - 1 && dictCount > 0 {
				switch tokens[i].Type {
				case DICTIONARY:
					dictCount++
				case END_OF_DICTIONARY:
					dictCount--
				}
				i++
			}
			i--
			item, err := ParseDict(tokens[dictStart:i + 1])
			if err != nil {
				return nil, err
			}
			list = append(list, item)
		}
		i++
	}
	return list, nil
}

func ParseDict(tokens []Token) (map[string]interface{}, error) {
	dict := make(map[string]interface{})
	if tokens[0].Type != DICTIONARY || tokens[len(tokens) - 1].Type != END_OF_DICTIONARY {
		return nil, errors.New("invalid dictionary form")
	}
	i := 1
	for i < len(tokens) - 1 {
		if tokens[i].Type != STRING {
			return nil, fmt.Errorf("key not string token_index: %d", i)
		}
		keyString := string(tokens[i].Value)
		i++
		switch tokens[i].Type {
		case LIST:
			listStart := i
			listCount := 1
			i++
			for i < len(tokens) - 1 && listCount > 0 {
				switch tokens[i].Type {
				case LIST:
					listCount++
				case END_OF_LIST:
					listCount--
				}
				i++
			}
			i--
			item, err := ParseList(tokens[listStart: i + 1])
			if err != nil {
				return nil, err
			}
			dict[keyString] = item
		case DICTIONARY:
			dictStart := i
			dictCount := 1
			i++
			for i < len(tokens) - 1 && dictCount > 0 {
				switch tokens[i].Type {
				case DICTIONARY:
					dictCount++
				case END_OF_DICTIONARY:
					dictCount--
				}
				i++
			}
			i--
			item, err := ParseDict(tokens[dictStart: i + 1])
			if err != nil {
				return nil, err
			}
			dict[keyString] = item
		case STRING:
			item, err := ParseString(tokens[i])
			if err != nil {
				return nil, err
			}
			dict[keyString] = item
		case INTEGER:
			item, err := ParseInteger(tokens[i])
			if err != nil {
				return nil, err
			}
			dict[keyString] = item
		}
		i++
	}
	return dict, nil
}

func Parse(data *[]byte) (map[string]interface{}, error) {
	tokens, err := Tokenize(data)
	if err != nil {
		return nil, err
	}
	dict, err := ParseDict(tokens)
	if err != nil {
		return nil, err
	}
	return dict, nil
}

func bEncodeInt(val int64) string {
	return fmt.Sprintf("i%de", val)
}

func bEncodeString(val string) string {
	return fmt.Sprintf("%d:%s", len(val), val)
}

func bEncodeList(list []interface{}) (string, error) {
	bString := "l"
	for _, item := range list {
		switch v := item.(type) {
		case string:
			bString += bEncodeString(v)
		case int64:
			bString += bEncodeInt(v)
		case []interface{}:
			listString, err := bEncodeList(v)
			if err != nil {
				return "", err
			}
			bString += listString
		case map[string]interface{}:
			dictString, err := BEncode(v)
			if err != nil {
				return "", err
			}
			bString += dictString
		}
	}
	bString += "e"
	return bString, nil
}

func BEncode(dict map[string]interface{}) (string, error) {
	bString := "d"
	for key, value := range dict {
		bString += bEncodeString(key)
		switch v := value.(type) {
		case string:
			bString += bEncodeString(v)
		case int64:
			bString += bEncodeInt(v)
		case []interface{}:
			listString, err := bEncodeList(v)
			if err != nil {
				return "", err
			}
			bString += listString
		case map[string]interface{}:
			dictString, err := BEncode(v)
			if err != nil {
				return "", err
			}
			bString += dictString
		}
	}
	bString += "e"
	return bString, nil
}

func GetInfoHash(data *[]byte) ([]byte, error) {
	//fmt.Println("Getting info hash...")
	sData := (*data)
	i := 0
	searchStr := "4:infod"
	curStr := string(sData[i:i+7])
	for i < len(sData) - 7 && curStr != searchStr{
		i++
		curStr = string(sData[i:i+7])
	}
	dictStart := i + 6
	contCount := 1
	i += 7
	for i < len(sData) && contCount > 0 {
		if sData[i] >= '0' && sData[i] <= '9' {
			j := i + 1
			for sData[j] >= '0' && sData[j] <= '9' {
				j++
			}
			if sData[j] == ':' {
				strLength, err := strconv.ParseInt(string(sData[i:j]), 10, 32)
				if err == nil {
					i = j + int(strLength) + 1
				} else {
					return nil, err
				}
			}
		}
		switch sData[i] {
		case 'i':
			for sData[i] != 'e' {
				i++
			}
			i++
		case 'd':
			contCount++
			i++
		case 'l':
			contCount++
			i++
		case 'e':
			contCount--
			i++
		}
	}
	//fmt.Printf("info start: %d, info end: %d\n", dictStart, i)
	sInfo := sData[dictStart: i]
	hasher := sha1.New()
	hasher.Write([]byte(sInfo))
	infoHash := hasher.Sum(nil)
	return infoHash, nil
}