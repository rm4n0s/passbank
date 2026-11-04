package kdbx

import (
	"context"
	"crypto/sha256"
	"io"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
)

// hashedBlockSize is the size of the blocks of the hashed block stream: 1 MiB, as KeePass writes.
const hashedBlockSize = 1 << 20

// encode3 writes a KDBX 2.0, 3.0 or 3.1 file (SPEC §4.5): the header with the
// seeds, the stream start bytes and the SHA-256 of the header repeated in the XML,
// then the encrypted payload, which is the start bytes and the hashed block
// stream of the compressed XML. The protected values are encrypted with Salsa20.
func encode3(ctx context.Context, db *model.Database, version Version, settings EncryptionSettings, creds crypto.Credentials, random io.Reader) ([]byte, error) {
	if db == nil || db.Root == nil {
		return nil, errors.New("DatabaseInvalid", "the database has no root group")
	}
	if err := Supports(version, settings); err != nil {
		return nil, err
	}
	cipher, err := crypto.LookupCipher(settings.CipherID)
	if err != nil {
		return nil, err
	}
	if settings.Compression != CompressionNone && settings.Compression != CompressionGZip {
		return nil, errors.New("CompressionUnsupported", "Passbank cannot write this compression", "compression", uint32(settings.Compression))
	}
	composite, err := crypto.CompositeKey(creds)
	if err != nil {
		return nil, err
	}
	defer composite.Wipe()

	kdf := settings.KDF
	var masterSeed, iv, streamKey, startBytes []byte
	for _, r := range []struct {
		dst *[]byte
		n   int
	}{{&masterSeed, seedLength}, {&iv, cipher.IVSize()}, {&kdf.Seed, seedLength}, {&streamKey, seedLength}, {&startBytes, startBytesLength}} {
		if *r.dst, err = randomBytes(random, r.n); err != nil {
			return nil, err
		}
	}
	defer clear(streamKey)
	if err := kdf.Validate(); err != nil {
		return nil, err
	}

	header := header3Bytes(version, cipher, settings.Compression, masterSeed, iv, kdf, streamKey, startBytes)
	headerHash := sha256.Sum256(header)
	transformed, err := kdf.Derive(ctx, composite.Reveal())
	if err != nil {
		return nil, err
	}
	defer clear(transformed)
	keyHash := sha256.New()
	keyHash.Write(masterSeed)
	keyHash.Write(transformed)
	cipherKey := keyHash.Sum(nil)
	defer clear(cipherKey)

	stream, err := crypto.NewInnerStream(crypto.InnerSalsa20, streamKey)
	if err != nil {
		return nil, err
	}
	payload, _, err := marshalXML(db, xmlSpec{
		Version: version, Stream: stream, HeaderHash: headerHash[:], CompressBinaries: settings.Compression == CompressionGZip,
	})
	if err != nil {
		return nil, err
	}
	defer clear(payload)
	if settings.Compression == CompressionGZip {
		payload = gzipBytes(payload)
		defer clear(payload)
	}
	plain := append(append([]byte(nil), startBytes...), writeHashedBlocks(payload, hashedBlockSize)...)
	defer clear(plain)
	ciphertext, err := cipher.Encrypt(cipherKey, iv, plain)
	if err != nil {
		return nil, err
	}
	return append(header, ciphertext...), nil
}

// header3Field encodes a field of the header of KDBX 2.x and 3.x: the ID, two
// bytes of length and the data.
func header3Field(id byte, data []byte) []byte {
	return append([]byte{id, byte(len(data)), byte(len(data) >> 8)}, data...)
}

// header3Bytes writes the outer header of a KDBX 2.x or 3.x file, signatures and
// version first and the end-of-header field last.
func header3Bytes(version Version, cipher crypto.Cipher, comp Compression, masterSeed, iv []byte, kdf crypto.KDF, streamKey, startBytes []byte) []byte {
	out := u32(signature1)
	out = append(out, u32(signature2KDBX)...)
	out = append(out, byte(version.Minor), byte(version.Minor>>8), byte(version.Major), byte(version.Major>>8))
	id := cipher.ID()
	out = append(out, header3Field(fieldCipherID, id[:])...)
	out = append(out, header3Field(fieldCompression, u32(uint32(comp)))...)
	out = append(out, header3Field(fieldMasterSeed, masterSeed)...)
	out = append(out, header3Field(fieldTransformSeed, kdf.Seed)...)
	out = append(out, header3Field(fieldTransformRounds, u64(kdf.Rounds))...)
	out = append(out, header3Field(fieldEncryptionIV, iv)...)
	out = append(out, header3Field(fieldProtectedStreamKey, streamKey)...)
	out = append(out, header3Field(fieldStreamStartBytes, startBytes)...)
	out = append(out, header3Field(fieldInnerRandomStream, u32(crypto.InnerSalsa20))...)
	return append(out, header3Field(fieldEnd, []byte("\r\n\r\n"))...)
}
