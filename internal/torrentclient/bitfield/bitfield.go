package bitfield

import (
	"errors"
	"fmt"
)

type BitField struct {
	Bytes []byte
	len int64
	Full bool
	NumSet int64
}

func LoadBytes(bytes []byte, len int64) *BitField {
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

func New(len int64) *BitField {
	var byteLen int64
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

func (bf *BitField) Print() {
	for _, b := range bf.Bytes {
		fmt.Printf("%08b ", b)
	}
	fmt.Println()
}