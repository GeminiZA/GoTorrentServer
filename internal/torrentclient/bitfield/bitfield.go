package bitfield

import (
	"encoding/hex"
	"errors"
	"fmt"
)

type Bitfield struct {
	Bytes    []byte
	len      int64
	Complete bool
	Empty    bool
	NumSet   int64
}

func FromBytes(bytes []byte, Len int64) *Bitfield {
	bf := Bitfield{Bytes: make([]byte, len(bytes)), len: Len}
	copy(bf.Bytes, bytes)
	for _, b := range bytes { // Count on bits
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
	bf.Empty = bf.NumSet == 0
	bf.Complete = bf.NumSet == bf.len
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
	bf := Bitfield{Bytes: []byte{}, len: len, Complete: false, Empty: true}
	for i := int64(0); i < byteLen; i++ {
		bf.Bytes = append(bf.Bytes, 0)
	}
	bf.NumSet = 0
	return &bf
}

func (bf *Bitfield) Len() int64 {
	return bf.len
}

func (bf *Bitfield) FirstOff() int64 {
	for i := range bf.Bytes {
		if bf.Bytes[i] != 0xFF {
			for j := 0; j < 8; j++ {
				if (bf.Bytes[i] & (1 << (7 - j))) == 0 {
					return int64(i*8) + int64(j)
				}
			}
		}
	}
	return -1
}

func (bf *Bitfield) Clone() *Bitfield {
	return FromBytes(bf.Bytes, bf.len)
}

func (bf *Bitfield) SetBit(index int64) error {
	if index > bf.len {
		return errors.New("out of bounds")
	}
	byteIndex := index / 8
	bitIndex := index % 8
	if bf.Bytes[byteIndex]&(1<<(7-bitIndex)) == 0 {
		bf.Bytes[byteIndex] = bf.Bytes[byteIndex] | (1 << (7 - bitIndex))
		bf.NumSet++
	}
	bf.Complete = bf.NumSet == bf.len
	return nil
}

func (bf *Bitfield) GetBit(index int64) bool {
	byteIndex := index / 8
	bitIndex := index % 8
	return (1 & (bf.Bytes[byteIndex] >> (7 - bitIndex))) == 1
}

func (bf *Bitfield) ResetBit(index int64) error {
	if index > bf.len {
		return errors.New("out of bounds")
	}
	byteIndex := index / 8
	bitIndex := index % 8
	if bf.Bytes[byteIndex]>>(7-bitIndex) == 1 {
		bf.NumSet--
		bf.Bytes[byteIndex] = bf.Bytes[byteIndex] & ^(1 << (7 - bitIndex))
		bf.Complete = bf.NumSet == bf.len
		bf.Empty = bf.NumSet == 0
	}
	return nil
}

func (bf *Bitfield) ResetAll() {
	for i := range bf.Bytes {
		bf.Bytes[i] = 0
	}
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

func (bfA *Bitfield) HasAll(bfB *Bitfield) bool {
	if bfA.len != bfB.len {
		return false
	}
	for i := range bfA.Bytes {
		if bfA.Bytes[i] == bfB.Bytes[i] {
			continue
		} else {
			for j := 0; j < 8; j++ {
				if ((1 & (bfB.Bytes[i] >> j)) == 1) && ((1 & (bfA.Bytes[i] >> j)) != 1) {
					return false
				}
			}
		}
	}
	return true
}

func (bfA *Bitfield) NumDiff(bfB *Bitfield) int64 {
	diffCount := int64(0)
	for i := range bfA.Bytes {
		diff := bfA.Bytes[i] & ^bfB.Bytes[i]
		for diff != 0 {
			diff &= diff - 1
			diffCount++
		}
	}
	return diffCount
}

func (bf *Bitfield) Print() string {
	msg := fmt.Sprintf("Bitfield (%d): ", bf.len)
	for _, b := range bf.Bytes {
		msg += fmt.Sprintf("%08b ", b)
	}
	msg += "\n"
	return msg
}
