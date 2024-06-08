package torrentfile

import (
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

func ParseInteger(token Token) (uint64, error) {
	retInt, err := strconv.ParseUint(string(token.Value), 10, 64)
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

