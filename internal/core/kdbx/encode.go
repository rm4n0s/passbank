package kdbx

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"io"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
)

// EncryptionSettings say how a file is encrypted and compressed. The seeds, the
// IV and the inner stream key are not settings: they are new random values on
// every write (SPEC §5).
type EncryptionSettings struct {
	// CipherID is the outer cipher.
	CipherID model.UUID
	// KDF is the key-derivation function with its parameters; its Seed is ignored.
	KDF crypto.KDF
	// Compression of the payload.
	Compression Compression
}

// SettingsOf returns the encryption settings of a file that was read.
func SettingsOf(info Info) EncryptionSettings {
	kdf := info.KDF
	kdf.Seed = nil
	return EncryptionSettings{CipherID: info.CipherID, KDF: kdf, Compression: info.Compression}
}

// The defaults of SPEC §5 (A5).
const (
	// DefaultArgon2Memory is 64 MiB.
	DefaultArgon2Memory = 64 << 20
	// DefaultArgon2Iterations is the number of passes of Argon2.
	DefaultArgon2Iterations = 10
	// DefaultArgon2Parallelism is the number of lanes of Argon2.
	DefaultArgon2Parallelism = 2
	// DefaultAESRounds is the number of rounds of the AES-KDF for KDBX 2.x and 3.x.
	DefaultAESRounds = 2_000_000
)

// DefaultSettings returns the settings for a new file of the version: AES-256 and
// GZip, with Argon2id for KDBX 4 and the AES-KDF for KDBX 2.x and 3.x.
func DefaultSettings(version Version) EncryptionSettings {
	s := EncryptionSettings{CipherID: crypto.CipherAES256, Compression: CompressionGZip}
	if version.Major >= 4 {
		s.KDF = crypto.KDF{
			ID: crypto.KDFArgon2id, Memory: DefaultArgon2Memory, Iterations: DefaultArgon2Iterations,
			Parallelism: DefaultArgon2Parallelism, Version: crypto.Argon2Version,
		}
	} else {
		s.KDF = crypto.KDF{ID: crypto.KDFAES, Rounds: DefaultAESRounds}
	}
	return s
}

// The size of the random values of a new file.
const (
	seedLength      = 32
	innerKeyLength4 = 64
	blockSize4      = 1 << 20
)

// Encode writes the database as the bytes of a KDBX file of the version, encrypted
// with the settings and the credentials. The database is not changed. Fields that
// the version cannot hold are left out without notice: the caller decides before
// that whether it may (SPEC §5, Losses). The credentials stay as they are; the
// caller wipes them.
func Encode(ctx context.Context, db *model.Database, version Version, settings EncryptionSettings, creds crypto.Credentials) ([]byte, error) {
	return encode(ctx, db, version, settings, creds, rand.Reader)
}

// encode is Encode with the source of the random values, so that a test can make
// it fail.
func encode(ctx context.Context, db *model.Database, version Version, settings EncryptionSettings, creds crypto.Credentials, random io.Reader) ([]byte, error) {
	if version.Major != 4 || version.Minor > 1 {
		return nil, errors.New("VersionUnsupported", "Passbank cannot write this KDBX version", "version", version.String())
	}
	if db == nil || db.Root == nil {
		return nil, errors.New("DatabaseInvalid", "the database has no root group")
	}
	cipher, err := crypto.LookupCipher(settings.CipherID)
	if err != nil {
		return nil, err
	}
	if settings.Compression != CompressionNone && settings.Compression != CompressionGZip {
		return nil, errors.New("CompressionUnsupported", "Passbank cannot write this compression", "compression", uint32(settings.Compression))
	}
	kdf := settings.KDF
	if kdf.ID == crypto.KDFArgon2d || kdf.ID == crypto.KDFArgon2id {
		kdf.Version = crypto.Argon2Version
	}
	composite, err := crypto.CompositeKey(creds)
	if err != nil {
		return nil, err
	}
	defer composite.Wipe()

	masterSeed, err := randomBytes(random, seedLength)
	if err != nil {
		return nil, err
	}
	iv, err := randomBytes(random, cipher.IVSize())
	if err != nil {
		return nil, err
	}
	if kdf.Seed, err = randomBytes(random, seedLength); err != nil {
		return nil, err
	}
	innerKey, err := randomBytes(random, innerKeyLength4)
	if err != nil {
		return nil, err
	}
	defer clear(innerKey)
	if err := kdf.Validate(); err != nil {
		return nil, err
	}

	header := header4Bytes(version, cipher, settings.Compression, masterSeed, iv, kdf, db.PublicCustomData)
	transformed, err := kdf.Derive(ctx, composite.Reveal())
	if err != nil {
		return nil, err
	}
	defer clear(transformed)
	base := hmacBaseKey(masterSeed, transformed)
	defer clear(base)
	keyHash := sha256.New()
	keyHash.Write(masterSeed)
	keyHash.Write(transformed)
	cipherKey := keyHash.Sum(nil)
	defer clear(cipherKey)

	stream, err := crypto.NewInnerStream(crypto.InnerChaCha20, innerKey)
	if err != nil {
		return nil, err
	}
	xmlBytes, binaries, err := marshalXML(db, xmlSpec{Version: version, Stream: stream})
	if err != nil {
		return nil, err
	}
	defer clear(xmlBytes)
	payload := encodeInnerHeader(crypto.InnerChaCha20, innerKey, binaries)
	for _, b := range binaries {
		b.Data.Wipe()
	}
	payload = append(payload, xmlBytes...)
	defer clear(payload)
	if settings.Compression == CompressionGZip {
		payload = gzipBytes(payload)
		defer clear(payload)
	}
	ciphertext, err := cipher.Encrypt(cipherKey, iv, payload)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(header)
	out := make([]byte, 0, len(header)+64+len(ciphertext)+len(ciphertext)/blockSize4*36+72)
	out = append(out, header...)
	out = append(out, sum[:]...)
	out = append(out, headerHMAC(base, header)...)
	return append(out, writeBlocks(base, ciphertext, blockSize4)...), nil
}

// randomBytes reads n random bytes.
func randomBytes(r io.Reader, n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, errors.NewErr("RandomFailed", err)
	}
	return b, nil
}

// header4Bytes writes the outer header of a KDBX 4 file, signatures and version
// first and the end-of-header field last.
func header4Bytes(version Version, cipher crypto.Cipher, comp Compression, masterSeed, iv []byte, kdf crypto.KDF, publicCustomData []model.PublicCustomDataItem) []byte {
	out := u32(signature1)
	out = append(out, u32(signature2KDBX)...)
	out = append(out, byte(version.Minor), byte(version.Minor>>8), byte(version.Major), byte(version.Major>>8))
	id := cipher.ID()
	out = append(out, field(fieldCipherID, id[:])...)
	out = append(out, field(fieldCompression, u32(uint32(comp)))...)
	out = append(out, field(fieldMasterSeed, masterSeed)...)
	out = append(out, field(fieldEncryptionIV, iv)...)
	out = append(out, field(fieldKDFParameters, encodeVariant(kdfToVariant(kdf)...))...)
	if len(publicCustomData) > 0 {
		items := make([]variantItem, len(publicCustomData))
		for i, it := range publicCustomData {
			items[i] = variantItem{Key: it.Key, Type: it.Type, Value: it.Data}
		}
		out = append(out, field(fieldPublicCustomData, encodeVariant(items...))...)
	}
	return append(out, field(fieldEnd, []byte("\r\n\r\n"))...)
}

// kdfToVariant is the KDF parameters of the header: the reverse of kdfFromVariant,
// for a function that Validate accepted. The AES-KDF always carries the ID
// c9d9f39a-…, which every program reads.
func kdfToVariant(k crypto.KDF) []variantItem {
	if k.ID == crypto.KDFAES {
		return []variantItem{
			{"$UUID", variantBytes, crypto.KDFAES[:]}, {"S", variantBytes, k.Seed}, {"R", variantUInt64, u64(k.Rounds)},
		}
	}
	return []variantItem{
		{"$UUID", variantBytes, k.ID[:]}, {"S", variantBytes, k.Seed}, {"P", variantUInt32, u32(k.Parallelism)},
		{"M", variantUInt64, u64(k.Memory)}, {"I", variantUInt64, u64(k.Iterations)}, {"V", variantUInt32, u32(k.Version)},
	}
}

// encodeInnerHeader writes the inner header of KDBX 4: the inner stream, its key,
// the attachments (a flag byte for "protected" and the content), and the end.
func encodeInnerHeader(streamID uint32, key []byte, binaries []innerBinaryData) []byte {
	out := field(innerRandomStreamID, u32(streamID))
	out = append(out, field(innerRandomStreamKey, key)...)
	for _, b := range binaries {
		flag := byte(0)
		if b.Protected {
			flag = 1
		}
		out = append(out, innerBinary)
		out = append(out, u32(uint32(1+b.Data.Len()))...)
		out = append(out, flag)
		out = append(out, b.Data.Reveal()...)
	}
	return append(out, field(innerEnd, nil)...)
}

// gzipBytes compresses data.
func gzipBytes(data []byte) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(data) // writing to a bytes.Buffer cannot fail
	_ = zw.Close()
	return buf.Bytes()
}
