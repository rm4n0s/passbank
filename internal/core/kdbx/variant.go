package kdbx

import (
	"encoding/binary"

	"github.com/rm4n0s/errors"
)

// The types of the values of a VariantDictionary (SPEC §4.1).
const (
	variantUInt32 = 0x04
	variantUInt64 = 0x05
	variantBool   = 0x08
	variantInt32  = 0x0C
	variantInt64  = 0x0D
	variantString = 0x18
	variantBytes  = 0x42
)

// Limits on what a VariantDictionary may hold; the file is not trusted.
const (
	maxVariantItems = 1024
	maxVariantKey   = 1024
	maxVariantValue = 1 << 20
)

// variantItem is one entry of a VariantDictionary: a key, the type byte, and the
// value as it is stored (little-endian numbers, UTF-8 strings, raw bytes).
type variantItem struct {
	Key   string
	Type  byte
	Value []byte
}

// parseVariantDictionary reads the VariantDictionary of KDBX 4 (the KDF
// parameters and the public custom data): a version (two bytes, 0x0100 for
// every file that exists, only the high byte matters), items of a type byte, a
// key and a value, and a zero type byte at the end.
func parseVariantDictionary(data []byte) ([]variantItem, error) {
	if len(data) < 3 {
		return nil, errors.New("FileCorrupt", "a variant dictionary of the header is too short", "length", len(data))
	}
	if data[1] != 0x01 {
		return nil, errors.New("FileCorrupt", "a variant dictionary of the header has an unknown version", "version", binary.LittleEndian.Uint16(data[0:2]))
	}
	var items []variantItem
	off := 2
	for {
		if off >= len(data) {
			return nil, errors.New("FileCorrupt", "a variant dictionary of the header is cut off")
		}
		typ := data[off]
		off++
		if typ == 0 {
			return items, nil
		}
		if len(items) >= maxVariantItems {
			return nil, errors.New("FileCorrupt", "a variant dictionary of the header has too many items", "max", maxVariantItems)
		}
		keyLen, ok := readLength(data, &off, maxVariantKey)
		if !ok || off+keyLen > len(data) {
			return nil, errors.New("FileCorrupt", "a variant dictionary of the header has a key that is too long or cut off")
		}
		key := string(data[off : off+keyLen])
		off += keyLen
		valLen, ok := readLength(data, &off, maxVariantValue)
		if !ok || off+valLen > len(data) {
			return nil, errors.New("FileCorrupt", "a variant dictionary of the header has a value that is too long or cut off", "key", key)
		}
		value := data[off : off+valLen]
		off += valLen
		if err := checkVariantSize(typ, key, len(value)); err != nil {
			return nil, err
		}
		items = append(items, variantItem{Key: key, Type: typ, Value: value})
	}
}

// readLength reads a four-byte length at *off, moves *off past it, and reports
// whether it was there and at most max.
func readLength(data []byte, off *int, max int) (int, bool) {
	if *off+4 > len(data) {
		return 0, false
	}
	n := binary.LittleEndian.Uint32(data[*off : *off+4])
	*off += 4
	if n > uint32(max) {
		return 0, false
	}
	return int(n), true
}

// checkVariantSize checks that a value has the size its type needs.
func checkVariantSize(typ byte, key string, n int) error {
	var want int
	switch typ {
	case variantUInt32, variantInt32:
		want = 4
	case variantUInt64, variantInt64:
		want = 8
	case variantBool:
		want = 1
	case variantString, variantBytes:
		return nil
	default:
		return errors.New("FileCorrupt", "a variant dictionary of the header has a value of an unknown type", "key", key, "type", typ)
	}
	if n != want {
		return errors.New("FileCorrupt", "a variant dictionary of the header has a value of the wrong size", "key", key, "type", typ, "size", n)
	}
	return nil
}

// variantDict looks the items up by key.
type variantDict map[string]variantItem

func newVariantDict(items []variantItem) variantDict {
	d := make(variantDict, len(items))
	for _, it := range items {
		d[it.Key] = it
	}
	return d
}

// uint64Of returns the value of a UInt64 item, or false.
func (d variantDict) uint64Of(key string) (uint64, bool) {
	it, ok := d[key]
	if !ok || it.Type != variantUInt64 {
		return 0, false
	}
	return binary.LittleEndian.Uint64(it.Value), true
}

// uint32Of returns the value of a UInt32 item, or false.
func (d variantDict) uint32Of(key string) (uint32, bool) {
	it, ok := d[key]
	if !ok || it.Type != variantUInt32 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(it.Value), true
}

// bytesOf returns the value of a byte array item, or false.
func (d variantDict) bytesOf(key string) ([]byte, bool) {
	it, ok := d[key]
	if !ok || it.Type != variantBytes {
		return nil, false
	}
	return it.Value, true
}
