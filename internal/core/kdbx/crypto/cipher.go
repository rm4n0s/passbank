package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"

	"github.com/rm4n0s/errors"
	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/twofish" //nolint:staticcheck // KDBX files use Twofish (SPEC §4.1); the package is the only implementation

	"github.com/rm4n0s/passbank/internal/core/model"
)

// The UUIDs of the outer ciphers of a KDBX file (SPEC §4.1).
var (
	// CipherAES256 is AES-256 in CBC mode with PKCS#7 padding.
	CipherAES256 = model.UUID{0x31, 0xc1, 0xf2, 0xe6, 0xbf, 0x71, 0x43, 0x50, 0xbe, 0x58, 0x05, 0x21, 0x6a, 0xfc, 0x5a, 0xff}
	// CipherTwofish is Twofish with a 256-bit key in CBC mode with PKCS#7 padding.
	CipherTwofish = model.UUID{0xad, 0x68, 0xf2, 0x9f, 0x57, 0x6f, 0x4b, 0xb9, 0xa3, 0x6a, 0xd4, 0x7a, 0xf9, 0x65, 0x34, 0x6c}
	// CipherChaCha20 is ChaCha20 (RFC 8439, 12-byte nonce, block counter 0).
	CipherChaCha20 = model.UUID{0xd6, 0x03, 0x8a, 0x2b, 0x8b, 0x6f, 0x4c, 0xb5, 0xa5, 0x24, 0x33, 0x9a, 0x31, 0xdb, 0xb5, 0x9a}
)

// Cipher is an outer cipher: it encrypts the payload of a KDBX file.
type Cipher struct {
	id      model.UUID
	name    string
	keySize int
	ivSize  int
	// newBlock makes the block cipher of a CBC cipher; it is nil for a stream cipher.
	newBlock func(key []byte) (cipher.Block, error)
}

// ciphers are the ciphers Passbank supports.
var ciphers = []Cipher{
	{id: CipherAES256, name: "AES-256", keySize: 32, ivSize: aes.BlockSize, newBlock: aes.NewCipher},
	{id: CipherTwofish, name: "Twofish", keySize: 32, ivSize: twofish.BlockSize, newBlock: newTwofish},
	{id: CipherChaCha20, name: "ChaCha20", keySize: chacha20.KeySize, ivSize: chacha20.NonceSize},
}

// newTwofish is twofish.NewCipher with the signature of Cipher.newBlock.
func newTwofish(key []byte) (cipher.Block, error) { return twofish.NewCipher(key) }

// LookupCipher returns the cipher with the UUID, or CipherUnsupported.
func LookupCipher(id model.UUID) (Cipher, error) {
	for _, c := range ciphers {
		if c.id == id {
			return c, nil
		}
	}
	return Cipher{}, errors.New("CipherUnsupported", "the file uses a cipher that Passbank does not support", "cipher", id.String())
}

// ID returns the UUID of the cipher.
func (c Cipher) ID() model.UUID { return c.id }

// Name returns the name of the cipher, such as "AES-256".
func (c Cipher) Name() string { return c.name }

// KeySize returns the length of the key in bytes.
func (c Cipher) KeySize() int { return c.keySize }

// IVSize returns the length of the initialization vector (the nonce of ChaCha20)
// in bytes.
func (c Cipher) IVSize() int { return c.ivSize }

// Encrypt encrypts plain: CBC ciphers add PKCS#7 padding (always at least one
// byte), ChaCha20 adds none. key and iv must have the sizes of the cipher.
func (c Cipher) Encrypt(key, iv, plain []byte) ([]byte, error) {
	if len(key) != c.keySize || len(iv) != c.ivSize {
		return nil, errors.New("CipherKeyInvalid", "the key or the initialization vector has the wrong length",
			"cipher", c.name, "keyLength", len(key), "wantKeyLength", c.keySize, "ivLength", len(iv), "wantIVLength", c.ivSize)
	}
	if c.newBlock == nil {
		out := make([]byte, len(plain))
		s, err := chacha20.NewUnauthenticatedCipher(key, iv)
		if err != nil {
			return nil, errors.NewErr("CipherInitFailed", err).SetMetadata("cipher", c.name)
		}
		s.XORKeyStream(out, plain)
		return out, nil
	}
	block, err := c.newBlock(key)
	if err != nil {
		return nil, errors.NewErr("CipherInitFailed", err).SetMetadata("cipher", c.name)
	}
	n := block.BlockSize() - len(plain)%block.BlockSize()
	padded := append(append(make([]byte, 0, len(plain)+n), plain...), bytes.Repeat([]byte{byte(n)}, n)...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(padded, padded)
	return padded, nil
}

// Decrypt decrypts data and, for a CBC cipher, removes and checks the padding:
// data that is empty or not a whole number of blocks is CiphertextInvalid, and
// padding that is not valid PKCS#7 (what a wrong key usually gives) is
// PaddingInvalid.
func (c Cipher) Decrypt(key, iv, data []byte) ([]byte, error) {
	raw, err := c.DecryptRaw(key, iv, data)
	if err != nil {
		return nil, err
	}
	return c.Unpad(raw)
}

// DecryptRaw decrypts data and leaves the padding of a CBC cipher in place, so
// that a caller can check the content (the stream start bytes of KDBX 3) before
// it looks at the padding, which a wrong key makes invalid. Data that is empty
// or not a whole number of blocks is CiphertextInvalid.
func (c Cipher) DecryptRaw(key, iv, data []byte) ([]byte, error) {
	if len(key) != c.keySize || len(iv) != c.ivSize {
		return nil, errors.New("CipherKeyInvalid", "the key or the initialization vector has the wrong length",
			"cipher", c.name, "keyLength", len(key), "wantKeyLength", c.keySize, "ivLength", len(iv), "wantIVLength", c.ivSize)
	}
	if c.newBlock == nil {
		out := make([]byte, len(data))
		s, err := chacha20.NewUnauthenticatedCipher(key, iv)
		if err != nil {
			return nil, errors.NewErr("CipherInitFailed", err).SetMetadata("cipher", c.name)
		}
		s.XORKeyStream(out, data)
		return out, nil
	}
	block, err := c.newBlock(key)
	if err != nil {
		return nil, errors.NewErr("CipherInitFailed", err).SetMetadata("cipher", c.name)
	}
	size := block.BlockSize()
	if len(data) == 0 || len(data)%size != 0 {
		return nil, errors.New("CiphertextInvalid", "the encrypted data is not a whole number of blocks",
			"cipher", c.name, "length", len(data), "blockSize", size)
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	return out, nil
}

// Unpad removes the PKCS#7 padding that DecryptRaw left; a stream cipher has
// none. Padding that is not valid is PaddingInvalid.
func (c Cipher) Unpad(raw []byte) ([]byte, error) {
	if c.newBlock == nil {
		return raw, nil
	}
	size := c.ivSize // the block size of the CBC ciphers
	n := 0
	if len(raw) > 0 {
		n = int(raw[len(raw)-1])
	}
	if n == 0 || n > size || n > len(raw) || !bytes.Equal(raw[len(raw)-n:], bytes.Repeat([]byte{byte(n)}, n)) {
		return nil, errors.New("PaddingInvalid", "the padding of the decrypted data is not valid", "cipher", c.name)
	}
	return raw[:len(raw)-n], nil
}
