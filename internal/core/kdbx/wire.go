package kdbx

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

// encodeVariant encodes a VariantDictionary.
func encodeVariant(items ...variantItem) []byte {
	out := []byte{0x00, 0x01}
	for _, it := range items {
		out = append(out, it.Type)
		out = binary.LittleEndian.AppendUint32(out, uint32(len(it.Key)))
		out = append(out, it.Key...)
		out = binary.LittleEndian.AppendUint32(out, uint32(len(it.Value)))
		out = append(out, it.Value...)
	}
	return append(out, 0)
}

// u32 and u64 encode a number little-endian, as KDBX does.
func u32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
func u64(v uint64) []byte { return binary.LittleEndian.AppendUint64(nil, v) }

// field encodes a header field.
func field(id byte, data []byte) []byte {
	return append(append([]byte{id}, u32(uint32(len(data)))...), data...)
}

// writeBlocks writes the HMAC block stream of KDBX 4: the data in blocks of at
// most size bytes, each with its HMAC, ending with the block of size zero.
func writeBlocks(base, data []byte, size int) []byte {
	var out []byte
	for index := uint64(0); ; index++ {
		n := min(size, len(data))
		block := data[:n]
		data = data[n:]
		key := blockKey(base, index)
		m := hmac.New(sha256.New, key)
		var prefix [12]byte
		binary.LittleEndian.PutUint64(prefix[:8], index)
		binary.LittleEndian.PutUint32(prefix[8:], uint32(n))
		m.Write(prefix[:])
		m.Write(block)
		out = append(out, m.Sum(nil)...)
		clear(key)
		out = append(out, u32(uint32(n))...)
		out = append(out, block...)
		if n == 0 {
			return out
		}
	}
}

// writeHashedBlocks writes the hashed block stream of KDBX 2.x and 3.x: the data
// in blocks of at most size bytes, each with its index, SHA-256 and size, ending
// with the block of size zero whose hash is zeros.
func writeHashedBlocks(data []byte, size int) []byte {
	var out []byte
	for index := uint32(0); ; index++ {
		n := min(size, len(data))
		block := data[:n]
		data = data[n:]
		out = binary.LittleEndian.AppendUint32(out, index)
		if n == 0 {
			out = append(out, make([]byte, 32)...)
		} else {
			sum := sha256.Sum256(block)
			out = append(out, sum[:]...)
		}
		out = binary.LittleEndian.AppendUint32(out, uint32(n))
		out = append(out, block...)
		if n == 0 {
			return out
		}
	}
}
