package kdbx

import (
	"encoding/binary"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
)

// The IDs of the fields of the outer header of KDBX 2.x and 3.x that KDBX 4 has
// no field for (SPEC §4.5). The others are shared with KDBX 4.
const (
	fieldTransformSeed      = 5
	fieldTransformRounds    = 6
	fieldProtectedStreamKey = 8
	fieldStreamStartBytes   = 9
	fieldInnerRandomStream  = 10
)

// Lengths of the fields of the old header.
const (
	transformSeedLength = 32
	startBytesLength    = 32
)

// header3 is the outer header of a KDBX 2.x or 3.x file: the part that is common
// with KDBX 4, and what only the old format keeps in its header.
type header3 struct {
	*Header
	// InnerStreamKey is the key of the inner stream (the protected stream key).
	InnerStreamKey []byte
	// InnerStreamID says how protected values are encrypted.
	InnerStreamID uint32
	// StreamStartBytes are the first 32 bytes of the decrypted payload, which tell
	// a right key from a wrong one.
	StreamStartBytes []byte
}

// parseHeader3 reads the outer header of a KDBX 2.x or 3.x file whose
// signatures and version are in data[:12]. It returns the header and the offset
// just after it (the end-of-header field included), which is the length of the
// part of the file that Meta/HeaderHash covers. A field it does not know is kept
// in Header.Unknown; a known field twice, or a required one missing, is
// FileCorrupt.
func parseHeader3(data []byte, version Version) (*header3, int, error) {
	h := &header3{Header: &Header{Version: version}}
	seen := map[byte]bool{}
	off := signatureSize
	for {
		if off+3 > len(data) {
			return nil, 0, errors.New("FileCorrupt", "the header of the file is cut off")
		}
		id := data[off]
		size := int(binary.LittleEndian.Uint16(data[off+1 : off+3]))
		off += 3
		if off+size > len(data) {
			return nil, 0, errors.New("FileCorrupt", "a field of the header is cut off", "field", id, "size", size)
		}
		field := data[off : off+size]
		off += size
		if id == fieldEnd {
			break
		}
		if seen[id] {
			return nil, 0, errors.New("FileCorrupt", "the header of the file has a field twice", "field", id)
		}
		seen[id] = true
		switch id {
		case fieldComment:
			h.Comment = field
		case fieldCipherID:
			if len(field) != len(h.CipherID) {
				return nil, 0, errors.New("FileCorrupt", "the cipher field of the header has the wrong length", "length", len(field))
			}
			copy(h.CipherID[:], field)
		case fieldCompression:
			if len(field) != 4 {
				return nil, 0, errors.New("FileCorrupt", "the compression field of the header has the wrong length", "length", len(field))
			}
			c := Compression(binary.LittleEndian.Uint32(field))
			if c != CompressionNone && c != CompressionGZip {
				return nil, 0, errors.New("CompressionUnsupported", "the file uses a compression that Passbank does not support", "compression", uint32(c))
			}
			h.Compression = c
		case fieldMasterSeed:
			if len(field) != masterSeedLength {
				return nil, 0, errors.New("FileCorrupt", "the master seed of the header has the wrong length", "length", len(field))
			}
			h.MasterSeed = field
		case fieldTransformSeed:
			if len(field) != transformSeedLength {
				return nil, 0, errors.New("FileCorrupt", "the transform seed of the header has the wrong length", "length", len(field))
			}
			h.KDF.Seed = field
		case fieldTransformRounds:
			if len(field) != 8 {
				return nil, 0, errors.New("FileCorrupt", "the transform rounds of the header have the wrong length", "length", len(field))
			}
			h.KDF.Rounds = binary.LittleEndian.Uint64(field)
		case fieldEncryptionIV:
			h.IV = field
		case fieldProtectedStreamKey:
			if len(field) == 0 {
				return nil, 0, errors.New("FileCorrupt", "the inner stream key of the header is empty")
			}
			h.InnerStreamKey = field
		case fieldStreamStartBytes:
			if len(field) != startBytesLength {
				return nil, 0, errors.New("FileCorrupt", "the stream start bytes of the header have the wrong length", "length", len(field))
			}
			h.StreamStartBytes = field
		case fieldInnerRandomStream:
			if len(field) != 4 {
				return nil, 0, errors.New("FileCorrupt", "the inner stream ID of the header has the wrong length", "length", len(field))
			}
			h.InnerStreamID = binary.LittleEndian.Uint32(field)
		default:
			h.Unknown = append(h.Unknown, HeaderField{ID: id, Data: field})
		}
	}
	for _, need := range []struct {
		id   byte
		name string
	}{{fieldCipherID, "cipher"}, {fieldCompression, "compression"}, {fieldMasterSeed, "master seed"},
		{fieldTransformSeed, "transform seed"}, {fieldTransformRounds, "transform rounds"}, {fieldEncryptionIV, "initialization vector"},
		{fieldProtectedStreamKey, "inner stream key"}, {fieldStreamStartBytes, "stream start bytes"}, {fieldInnerRandomStream, "inner stream ID"}} {
		if !seen[need.id] {
			return nil, 0, errors.New("FileCorrupt", "the header of the file has no "+need.name+" field", "field", need.id)
		}
	}
	h.KDF.ID = crypto.KDFAES
	return h, off, nil
}
