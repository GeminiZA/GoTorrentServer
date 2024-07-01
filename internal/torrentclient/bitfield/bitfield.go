package bitfield

import (
	"encoding/hex"
	"errors"
	"fmt"
)

type Bitfield struct {
	Bytes []byte
	len int64
	Full bool
	NumSet int64
}

func FromBytes(bytes []byte, len int64) *Bitfield {
	bf := Bitfield{Bytes: bytes, len: len}
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

func FromHex(bfHex string, Length int64) (*Bitfield, error) {
	byteLen := (Length + 7) / 8
	bytes, err := hex.DecodeString(bfHex)
	if err != nil {
		return nil, err
	}
	if int64(len(bytes)) != byteLen {
		return nil, errors.New("hex string length does not match bytes")
	}
	bf := Bitfield{Bytes: bytes, len: Length}
	for _, b := range bytes {
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
	return &bf, nil
}

func New(len int64) *Bitfield {
	byteLen := (len + 7) / 8
	bf := Bitfield{Bytes: []byte{}, len: len, Full: false}
	for i := int64(0); i < byteLen; i++ {
		bf.Bytes = append(bf.Bytes, 0)
	}
	bf.NumSet = 0
	return &bf
}

func (bf *Bitfield) Len() int64 {
	return bf.len
}

func (bf *Bitfield) ToHex() string {
	return fmt.Sprintf("%x", bf.Bytes)
}

func (bf *Bitfield) FirstOff() int64 {
	for i := range bf.Bytes {
		if bf.Bytes[i] != 0xFF {
			for j := 0; j < 8; j++ {
				if (bf.Bytes[i] & (1 << (7 - j)))  == 0 {
					return int64(i * 8) + int64(j)
				}
			}
		}
	}
	return -1
}

func (bf *Bitfield) SetBit(index int64) error {
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

func (bf *Bitfield) GetBit(index int64) bool {
	byteIndex := index / 8
	bitIndex := index % 8
	return (1 & (bf.Bytes[byteIndex] >> (7 - bitIndex))) == 1
}

func (bf *Bitfield) Complete() bool {
	for _, b := range bf.Bytes {
		if !(b == 0xFF) {
			return false
		}
	}
	return true
}

func (bfA *Bitfield) And(bfB *Bitfield) *Bitfield {
	len := min(bfA.len, bfB.len)
	byteLen := (len + 7) / 8
	bytes := make([]byte, byteLen)
	for i := range bytes {
		bytes[i] = bfA.Bytes[i] & bfB.Bytes[i]
	}
	return FromBytes(bytes, len)
}

func (bfA *Bitfield) Equals(bfB *Bitfield) bool {
	if bfA.len != bfB.len {
		return false
	}
	for i := range bfA.Bytes {
		if bfA.Bytes[i] != bfB.Bytes[i] {
			return false
		}
	}
	return true
}


func (bf *Bitfield) Print() {
	for _, b := range bf.Bytes {
		fmt.Printf("%08b ", b)
	}
	fmt.Println()
}