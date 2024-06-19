package bitfield

import (
	"errors"
	"fmt"
)

type BitField struct {
	Bytes []byte
	len uint32
	Full bool
	NumSet uint32
}

func LoadBytes(bytes []byte, len uint32) *BitField {
	bf := BitField{Bytes: bytes, len: len}
	bf.Full = bf.Complete()
	for _, b := range bytes { //Count on bits
		if b == 0xFF {
			bf.NumSet += 8
		} else {
			curByte := b
			for curByte != 0 {
				curByte &= curByte - 1
				bf.NumSet++
			}
		}
	}
	return &bf
}

func New(len uint32) *BitField {
	var byteLen uint32
	if len % 8 != 0 {
		byteLen = (len / 8) + 1
	} else {
		byteLen = len / 8
	}
	bf := BitField{Bytes: []byte{}, len: len, Full: false}
	for i := int64(0); i < byteLen; i++ {
		bf.Bytes = append(bf.Bytes, 0)
	}
	bf.NumSet = 0
	return &bf
}

func (bf *BitField) GetAll() []byte {
	return bf.Bytes
}

func (bf *BitField) Len() int64 {
	return bf.len
}

func (bf *BitField) SetBit(index int64) error {
	if index > int64(len(bf.Bytes))/8 {
		return errors.New("out of bounds")
	}
	byteIndex := index / 8
	bitIndex := index % 8
	bf.Bytes[byteIndex] = bf.Bytes[byteIndex] | (1 << (7 - bitIndex))
	bf.NumSet++
	fmt.Printf("Set bit: %d\n", index)
	return nil
}

func (bf *BitField) GetBit(index int64) bool {
	byteIndex := index / 8
	bitIndex := index % 8
	return (1 & (bf.Bytes[byteIndex] >> (7 - bitIndex))) == 1
}

func (bf *BitField) Complete() bool {
	for _, b := range bf.Bytes {
		if !(b == 0xFF) {
			return false
		}
	}
	return true
}

func (bfA *BitField) And(bfB *BitField) *BitField {
	len := min(bfA.len, bfB.len)
	var byteLen int64
	if len % 8 != 0 {
		byteLen = (len / 8) + 1
	} else {
		byteLen = len / 8
	}
	bytes := make([]byte, byteLen)
	for i := range bytes {
		bytes[i] = bfA.Bytes[i] & bfB.Bytes[i]
	}
	return LoadBytes(bytes, len)
}

func (bf *BitField) Print() {
	for _, b := range bf.Bytes {
		fmt.Printf("%08b ", b)
	}
	fmt.Println()
}