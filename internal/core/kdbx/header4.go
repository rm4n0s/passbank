package kdbx

import (
	"encoding/binary"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
)

// Compression is how the payload of a file is compressed.
type Compression uint32

// The values of Compression.
const (
	// CompressionNone stores the payload as it is.
	CompressionNone Compression = 0
	// CompressionGZip compresses the payload with gzip.
	CompressionGZip Compression = 1
)

// String names the compression.
func (c Compression) String() string {
	switch c {
	case CompressionNone:
		return "none"
	case CompressionGZip:
		return "GZip"
	}
	return "unknown"
}

// The IDs of the fields of the outer header of a KDBX 4 file.
const (
	fieldEnd              = 0
	fieldComment          = 1
	fieldCipherID         = 2
	fieldCompression      = 3
	fieldMasterSeed       = 4
	fieldEncryptionIV     = 7
	fieldKDFParameters    = 11
	fieldPublicCustomData = 12
)

// Limits on the header, which is read before anything is checked: a field of
// 4 MiB and a header of 8 MiB are far more than any real file has.
const (
	maxHeaderField = 4 << 20
	maxHeader      = 8 << 20
)

// masterSeedLength is the length of the master seed of KDBX 4.
const masterSeedLength = 32

// HeaderField is a field of the outer header that Passbank does not know.
type HeaderField struct {
	ID   byte
	Data []byte
}

// Header is what the outer header of a KDBX 4 file says: how the rest is
// encrypted and compressed.
type Header struct {
	// Version is the version of the file.
	Version Version
	// CipherID is the UUID of the outer cipher.
	CipherID model.UUID
	// Compression is how the payload is compressed.
	Compression Compression
	// MasterSeed and IV are the random values of the file.
	MasterSeed []byte
	IV         []byte
	// KDF holds the key-derivation function and its parameters.
	KDF crypto.KDF
	// PublicCustomData is the header field that plugins read without the key.
	PublicCustomData []model.PublicCustomDataItem
	// Comment is the (obsolete) comment field, if there is one.
	Comment []byte
	// Unknown are the fields that no version of Passbank knows; KeePass ignores
	// them, and so does Passbank, but it keeps them so that nothing is lost
	// unseen.
	Unknown []HeaderField
}

// parseHeader4 reads the outer header of a KDBX 4 file whose signatures and
// version are in data[:12]. It returns the header and the offset just after the
// header (the end-of-header field included), which is the length of the part of
// the file that the header hash and the header HMAC cover.
func parseHeader4(data []byte, version Version) (*Header, int, error) {
	h := &Header{Version: version}
	seen := map[byte]bool{}
	off := signatureSize
	for {
		if off+5 > len(data) {
			return nil, 0, errors.New("FileCorrupt", "the header of the file is cut off")
		}
		id := data[off]
		size := binary.LittleEndian.Uint32(data[off+1 : off+5])
		off += 5
		if size > maxHeaderField || off+int(size) > len(data) {
			return nil, 0, errors.New("FileCorrupt", "a field of the header is too long or cut off", "field", id, "size", size)
		}
		if off+int(size) > maxHeader {
			return nil, 0, errors.New("FileCorrupt", "the header of the file is too long", "max", maxHeader)
		}
		field := data[off : off+int(size)]
		off += int(size)
		if id == fieldEnd {
			break
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
		case fieldEncryptionIV:
			h.IV = field
		case fieldKDFParameters:
			items, err := parseVariantDictionary(field)
			if err != nil {
				return nil, 0, err
			}
			kdf, err := kdfFromVariant(newVariantDict(items))
			if err != nil {
				return nil, 0, err
			}
			h.KDF = kdf
		case fieldPublicCustomData:
			items, err := parseVariantDictionary(field)
			if err != nil {
				return nil, 0, err
			}
			for _, it := range items {
				h.PublicCustomData = append(h.PublicCustomData, model.PublicCustomDataItem{Key: it.Key, Type: it.Type, Data: it.Value})
			}
		default:
			h.Unknown = append(h.Unknown, HeaderField{ID: id, Data: field})
		}
	}
	for _, need := range []struct {
		id   byte
		name string
	}{{fieldCipherID, "cipher"}, {fieldCompression, "compression"}, {fieldMasterSeed, "master seed"},
		{fieldEncryptionIV, "initialization vector"}, {fieldKDFParameters, "key-derivation parameters"}} {
		if !seen[need.id] {
			return nil, 0, errors.New("FileCorrupt", "the header of the file has no "+need.name+" field", "field", need.id)
		}
	}
	return h, off, nil
}

// kdfFromVariant turns the KDF parameters of the header into a crypto.KDF. A
// function it does not know is returned with only its ID set, and
// crypto.KDF.Validate says KdfUnsupported; a parameter that is missing or of the
// wrong type is KdfParametersRejected.
func kdfFromVariant(d variantDict) (crypto.KDF, error) {
	idBytes, ok := d.bytesOf("$UUID")
	if !ok || len(idBytes) != 16 {
		return crypto.KDF{}, errors.New("FileCorrupt", "the key-derivation parameters have no function ID")
	}
	var k crypto.KDF
	copy(k.ID[:], idBytes)
	if k.ID == crypto.KDFAESKdbx4 {
		k.ID = crypto.KDFAES // the same function under the ID that KeePassXC writes
	}
	seed, haveSeed := d.bytesOf("S")
	switch k.ID {
	case crypto.KDFAES:
		rounds, haveRounds := d.uint64Of("R")
		if !haveSeed || !haveRounds {
			return crypto.KDF{}, errors.New("KdfParametersRejected", "the parameters of the AES-KDF lack the seed (S) or the rounds (R)")
		}
		k.Seed, k.Rounds = seed, rounds
	case crypto.KDFArgon2d, crypto.KDFArgon2id:
		iterations, haveI := d.uint64Of("I")
		memory, haveM := d.uint64Of("M")
		parallelism, haveP := d.uint32Of("P")
		version, haveV := d.uint32Of("V")
		if !haveSeed || !haveI || !haveM || !haveP || !haveV {
			return crypto.KDF{}, errors.New("KdfParametersRejected", "the parameters of Argon2 lack the salt (S), iterations (I), memory (M), parallelism (P) or version (V)")
		}
		for _, key := range []string{"K", "A"} { // the secret key and the associated data, which KDBX never sets
			if it, present := d[key]; present && len(it.Value) > 0 {
				return crypto.KDF{}, errors.New("KdfParametersRejected", "Argon2 with a secret key or associated data is not supported", "parameter", key)
			}
		}
		k.Seed, k.Iterations, k.Memory, k.Parallelism, k.Version = seed, iterations, memory, parallelism, version
	}
	return k, nil
}
