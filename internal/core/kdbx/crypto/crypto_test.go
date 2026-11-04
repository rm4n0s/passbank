package crypto

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"golang.org/x/crypto/salsa20"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/third_party/argon2"
)

// assertRoute fails the test unless err is an *errors.Error whose route contains route.
func assertRoute(t *testing.T, err error, route string) {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute(route) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), route)
	}
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sequence(start, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(start + i)
	}
	return b
}

// TestIDs checks the UUIDs against the strings of SPEC §4.1, so that a typo in a
// byte cannot go unnoticed.
func TestIDs(t *testing.T) {
	tests := []struct {
		name string
		id   model.UUID
		want string
	}{
		{"AES-256", CipherAES256, "31c1f2e6-bf71-4350-be58-05216afc5aff"},
		{"Twofish", CipherTwofish, "ad68f29f-576f-4bb9-a36a-d47af965346c"},
		{"ChaCha20", CipherChaCha20, "d6038a2b-8b6f-4cb5-a524-339a31dbb59a"},
		{"AES-KDF", KDFAES, "c9d9f39a-628a-4460-bf74-0d08c18a4fea"},
		{"Argon2d", KDFArgon2d, "ef636ddf-8c29-444b-91f7-a9a403e30a0c"},
		{"Argon2id", KDFArgon2id, "9e298b19-56db-4773-b23d-fc3ec6f0a1e6"},
	}
	for _, tt := range tests {
		if got := tt.id.String(); got != tt.want {
			t.Errorf("%s = %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestLookupCipher(t *testing.T) {
	tests := []struct {
		id      model.UUID
		name    string
		key, iv int
	}{
		{CipherAES256, "AES-256", 32, 16},
		{CipherTwofish, "Twofish", 32, 16},
		{CipherChaCha20, "ChaCha20", 32, 12},
	}
	for _, tt := range tests {
		c, err := LookupCipher(tt.id)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if c.ID() != tt.id || c.Name() != tt.name || c.KeySize() != tt.key || c.IVSize() != tt.iv {
			t.Errorf("%s: got %v %q key %d iv %d", tt.name, c.ID(), c.Name(), c.KeySize(), c.IVSize())
		}
	}
	_, err := LookupCipher(model.UUID{1})
	assertRoute(t, err, "LookupCipher.CipherUnsupported")
}

// TestCipherKnownAnswers checks each cipher against a value that does not come from
// this code: OpenSSL (AES-256-CBC), the Twofish paper (the 256-bit all-zero key)
// and RFC 8439 appendix A.1 (ChaCha20).
func TestCipherKnownAnswers(t *testing.T) {
	zero32, zero16, zero12 := make([]byte, 32), make([]byte, 16), make([]byte, 12)
	tests := []struct {
		name      string
		id        model.UUID
		key, iv   []byte
		plain     []byte
		wantCrypt string // the whole ciphertext, or its first block when firstOnly
		firstOnly bool
	}{
		{
			// openssl enc -aes-256-cbc -K 000102…1f -iv 0f0e…00
			name: "AES-256-CBC with padding", id: CipherAES256, key: sequence(0, 32),
			iv:        sequence(0, 16), // replaced below by the reversed IV
			plain:     []byte("hello passbank, 32 bytes of text!"),
			wantCrypt: "af5473dd2b2dc01a4cf016d8143495ee9953c412a1d141351185412e65d187a9def6058a824979e110b34f360559845c",
		},
		{
			name: "Twofish, all-zero 256-bit key", id: CipherTwofish, key: zero32, iv: zero16, plain: make([]byte, 16),
			wantCrypt: "57ff739d4dc92c1bd7fc01700cc8216f", firstOnly: true,
		},
		{
			name: "ChaCha20, RFC 8439 A.1 test vector 1", id: CipherChaCha20, key: zero32, iv: zero12, plain: make([]byte, 64),
			wantCrypt: "76b8e0ada0f13d90405d6ae55386bd28bdd219b8a08ded1aa836efcc8b770dc7da41597c5157488d7724e03fb8d84a376a43b8f41518a11cc387b669b2ee6586",
		},
	}
	// The OpenSSL IV of the first case is 0f0e0d…00.
	tests[0].iv = make([]byte, 16)
	for i := range tests[0].iv {
		tests[0].iv[i] = byte(15 - i)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := LookupCipher(tt.id)
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Encrypt(tt.key, tt.iv, tt.plain)
			if err != nil {
				t.Fatal(err)
			}
			want := unhex(t, tt.wantCrypt)
			if tt.firstOnly {
				got = got[:len(want)]
			}
			if !bytes.Equal(got, want) {
				t.Errorf("ciphertext = %x, want %x", got, want)
			}
			full, err := c.Encrypt(tt.key, tt.iv, tt.plain)
			if err != nil {
				t.Fatal(err)
			}
			back, err := c.Decrypt(tt.key, tt.iv, full)
			if err != nil || !bytes.Equal(back, tt.plain) {
				t.Errorf("round trip = %q, %v; want %q", back, err, tt.plain)
			}
		})
	}
}

func TestCipherRoundTripAllLengths(t *testing.T) {
	for _, id := range []model.UUID{CipherAES256, CipherTwofish, CipherChaCha20} {
		c, err := LookupCipher(id)
		if err != nil {
			t.Fatal(err)
		}
		key, iv := sequence(1, c.KeySize()), sequence(50, c.IVSize())
		for n := 0; n <= 70; n++ {
			plain := sequence(n, n)
			enc, err := c.Encrypt(key, iv, plain)
			if err != nil {
				t.Fatal(err)
			}
			if c.Name() != "ChaCha20" && (len(enc) <= n || len(enc)%16 != 0) {
				t.Errorf("%s: %d bytes encrypted to %d; CBC pads to a whole number of blocks, always adding some", c.Name(), n, len(enc))
			}
			if c.Name() == "ChaCha20" && len(enc) != n {
				t.Errorf("ChaCha20: %d bytes encrypted to %d", n, len(enc))
			}
			dec, err := c.Decrypt(key, iv, enc)
			if err != nil || !bytes.Equal(dec, plain) {
				t.Errorf("%s: %d bytes: round trip = %x, %v", c.Name(), n, dec, err)
			}
		}
	}
}

func TestCipherErrors(t *testing.T) {
	aes, err := LookupCipher(CipherAES256)
	if err != nil {
		t.Fatal(err)
	}
	key, iv := sequence(1, 32), sequence(1, 16)
	t.Run("wrong sizes", func(t *testing.T) {
		_, err := aes.Encrypt(key[:31], iv, nil)
		assertRoute(t, err, "Cipher.Encrypt.CipherKeyInvalid")
		_, err = aes.Decrypt(key, iv[:15], make([]byte, 16))
		assertRoute(t, err, "Cipher.Decrypt.CipherKeyInvalid")
	})
	t.Run("not whole blocks", func(t *testing.T) {
		for _, n := range []int{0, 1, 15, 17, 31} {
			_, err := aes.Decrypt(key, iv, make([]byte, n))
			assertRoute(t, err, "Cipher.Decrypt.CiphertextInvalid")
		}
	})
	t.Run("bad padding", func(t *testing.T) {
		good, err := aes.Encrypt(key, iv, []byte("abc"))
		if err != nil {
			t.Fatal(err)
		}
		// A wrong key gives garbage; almost always garbage padding. Try several keys
		// and require that every one that fails does so with PaddingInvalid.
		failed := 0
		for i := 0; i < 64; i++ {
			wrong := sequence(100+i, 32)
			if _, err := aes.Decrypt(wrong, iv, good); err != nil {
				assertRoute(t, err, "Cipher.Decrypt.PaddingInvalid")
				failed++
			}
		}
		if failed < 50 {
			t.Errorf("only %d of 64 wrong keys gave PaddingInvalid", failed)
		}
		// Forge each kind of bad padding: encrypt a block whose last bytes are wrong.
		for name, last := range map[string][]byte{
			"zero":           {0},
			"larger than 16": {17},
			"inconsistent":   {1, 2},
			"too long":       {16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 15},
		} {
			block := append(make([]byte, 16-len(last)), last...)
			forged, err := aes.encryptRaw(key, iv, block)
			if err != nil {
				t.Fatal(err)
			}
			_, err = aes.Decrypt(key, iv, forged)
			if err == nil {
				t.Errorf("%s: bad padding accepted", name)
				continue
			}
			assertRoute(t, err, "Cipher.Decrypt.PaddingInvalid")
		}
	})
}

// encryptRaw encrypts whole blocks in CBC mode without adding padding, to forge
// ciphertext with bad padding.
func (c Cipher) encryptRaw(key, iv, blocks []byte) ([]byte, error) {
	// Encrypt with padding and drop the padding block: the first len(blocks) bytes
	// are exactly the CBC encryption of blocks.
	enc, err := c.Encrypt(key, iv, blocks)
	if err != nil {
		return nil, err
	}
	return enc[:len(blocks)], nil
}

// TestAESKDF checks the AES-KDF against values computed with OpenSSL and Python:
// the key 00..1f and the seed 64..83, 1, 2 and 10 rounds of AES-256-ECB on each half,
// then SHA-256.
func TestAESKDF(t *testing.T) {
	key, seed := sequence(0, 32), sequence(100, 32)
	tests := []struct {
		rounds uint64
		want   string
	}{
		{1, "1cc3b4980297205189b3229e288a2ec9fcb6b5df1f6cda8c0dd21aaf77e46cd4"},
		{2, "b61bc6601f28b7a73d723d05507c589ef87f2fd3cc49fa7e644c3e5321cd29b8"},
		{10, "612bfc1b060c3effab06f14ed580ab926bbb2cae9dc9d75a45f3ff6b93e4d8f0"},
	}
	for _, tt := range tests {
		k := KDF{ID: KDFAES, Seed: seed, Rounds: tt.rounds}
		got, err := k.Derive(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(got) != tt.want {
			t.Errorf("%d rounds: %x, want %s", tt.rounds, got, tt.want)
		}
	}
	// More rounds than the cancellation check interval, to cover the loop.
	k := KDF{ID: KDFAES, Seed: seed, Rounds: 3*aesRoundsPerCheck + 5}
	a, err := k.Derive(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	k.Rounds = 3*aesRoundsPerCheck + 6
	b, err := k.Derive(context.Background(), key)
	if err != nil || bytes.Equal(a, b) {
		t.Errorf("one more round must change the result (%v)", err)
	}
	if !bytes.Equal(key, sequence(0, 32)) {
		t.Error("Derive changed the key it was given")
	}
}

// TestArgon2KDF checks that the KDF hands the parameters to the right Argon2: the
// result is that of the library with the memory in KiB.
func TestArgon2KDF(t *testing.T) {
	key, salt := sequence(1, 32), sequence(2, 16)
	for _, tt := range []struct {
		id  model.UUID
		run func(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte
	}{{KDFArgon2d, argon2.DKey}, {KDFArgon2id, argon2.IDKey}} {
		k := KDF{ID: tt.id, Seed: salt, Memory: 64 << 10, Iterations: 2, Parallelism: 2, Version: Argon2Version}
		got, err := k.Derive(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if want := tt.run(key, salt, 2, 64, 2, 32); !bytes.Equal(got, want) {
			t.Errorf("%s = %x, want %x", k.Name(), got, want)
		}
	}
	d := KDF{ID: KDFArgon2d, Seed: salt, Memory: 64 << 10, Iterations: 2, Parallelism: 2, Version: Argon2Version}
	id := d
	id.ID = KDFArgon2id
	a, errA := d.Derive(context.Background(), key)
	b, errB := id.Derive(context.Background(), key)
	if errA != nil || errB != nil || bytes.Equal(a, b) {
		t.Errorf("Argon2d and Argon2id must differ (%v, %v)", errA, errB)
	}
	if !bytes.Equal(key, sequence(1, 32)) {
		t.Error("Derive changed the key it was given")
	}
}

func TestKDFValidate(t *testing.T) {
	salt := sequence(0, 16)
	argon := func(mut func(*KDF)) KDF {
		k := KDF{ID: KDFArgon2id, Seed: salt, Memory: 64 << 20, Iterations: 2, Parallelism: 2, Version: Argon2Version}
		mut(&k)
		return k
	}
	aes := func(mut func(*KDF)) KDF {
		k := KDF{ID: KDFAES, Seed: sequence(0, 32), Rounds: 100}
		mut(&k)
		return k
	}
	bad := map[string]KDF{
		"AES seed too short":       aes(func(k *KDF) { k.Seed = k.Seed[:31] }),
		"AES zero rounds":          aes(func(k *KDF) { k.Rounds = 0 }),
		"AES too many rounds":      aes(func(k *KDF) { k.Rounds = MaxAESRounds + 1 }),
		"Argon2 version 1.0":       argon(func(k *KDF) { k.Version = 0x10 }),
		"Argon2 salt too short":    argon(func(k *KDF) { k.Seed = salt[:7] }),
		"Argon2 salt too long":     argon(func(k *KDF) { k.Seed = make([]byte, MaxArgon2SeedLength+1) }),
		"Argon2 no lanes":          argon(func(k *KDF) { k.Parallelism = 0 }),
		"Argon2 too many lanes":    argon(func(k *KDF) { k.Parallelism = MaxArgon2Parallelism + 1 }),
		"Argon2 no iterations":     argon(func(k *KDF) { k.Iterations = 0 }),
		"Argon2 too many passes":   argon(func(k *KDF) { k.Iterations = MaxArgon2Iterations + 1 }),
		"Argon2 too much memory":   argon(func(k *KDF) { k.Memory = MaxArgon2Memory + 1024 }),
		"Argon2 too little memory": argon(func(k *KDF) { k.Memory = 8*1024*2 - 1024 }),
		"Argon2 memory per lane":   argon(func(k *KDF) { k.Parallelism = 255; k.Memory = 8*1024*255 - 1024 }),
	}
	for name, k := range bad {
		assertRoute(t, k.Validate(), "KDF.Validate.KdfParametersRejected")
		t.Run(name, func(t *testing.T) {
			// Derive must refuse before it allocates or runs anything.
			start := time.Now()
			_, err := k.Derive(context.Background(), sequence(0, 32))
			assertRoute(t, err, "KDF.Derive->KDF.Validate.KdfParametersRejected")
			if time.Since(start) > time.Second {
				t.Errorf("refusing took %v", time.Since(start))
			}
		})
	}
	good := []KDF{
		aes(func(*KDF) {}), aes(func(k *KDF) { k.Rounds = MaxAESRounds }),
		argon(func(*KDF) {}), argon(func(k *KDF) { k.Memory = MaxArgon2Memory; k.Parallelism = 255; k.Iterations = MaxArgon2Iterations }),
		argon(func(k *KDF) { k.Memory = 8 * 1024 * 2; k.Seed = salt[:8] }), argon(func(k *KDF) { k.ID = KDFArgon2d }),
	}
	for i, k := range good {
		if err := k.Validate(); err != nil {
			t.Errorf("good[%d]: %v", i, err)
		}
	}
	unknown := KDF{ID: model.UUID{9}}
	assertRoute(t, unknown.Validate(), "KDF.Validate.KdfUnsupported")
	if unknown.Name() != "unknown" || (KDF{ID: KDFAES}).Name() != "AES-KDF" || (KDF{ID: KDFArgon2d}).Name() != "Argon2d" || (KDF{ID: KDFArgon2id}).Name() != "Argon2id" {
		t.Error("Name is wrong")
	}
	_, err := (KDF{ID: KDFAES, Seed: sequence(0, 32), Rounds: 1}).Derive(context.Background(), make([]byte, 31))
	assertRoute(t, err, "KDF.Derive.KdfKeyInvalid")
}

func TestKDFCancel(t *testing.T) {
	key := sequence(0, 32)
	t.Run("before starting", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := KDF{ID: KDFAES, Seed: sequence(0, 32), Rounds: 1}.Derive(ctx, key)
		assertRoute(t, err, "KDF.Derive.KdfCancelled")
		if !errors.Is(err, context.Canceled) {
			t.Error("the error must wrap context.Canceled")
		}
	})
	t.Run("AES-KDF in the middle", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := KDF{ID: KDFAES, Seed: sequence(0, 32), Rounds: MaxAESRounds}.Derive(ctx, key)
		assertRoute(t, err, "KDF.deriveAES.KdfCancelled")
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("cancelling took %v", d)
		}
	})
	t.Run("Argon2 in the middle", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()
		k := KDF{ID: KDFArgon2id, Seed: sequence(0, 16), Memory: 256 << 20, Iterations: 3, Parallelism: 1, Version: Argon2Version}
		start := time.Now()
		_, err := k.Derive(ctx, key)
		assertRoute(t, err, "KDF.deriveArgon2.KdfCancelled")
		if d := time.Since(start); d > time.Second {
			t.Errorf("abandoning took %v", d)
		}
	})
}

// TestInnerStreams checks the inner streams against values that do not come from
// this code.
func TestInnerStreams(t *testing.T) {
	const innerKey = "inner-stream-key-0123456789"
	t.Run("ChaCha20 against OpenSSL", func(t *testing.T) {
		// SHA-512 of the key gives the key and the nonce; the keystream is
		// openssl enc -chacha20 -K <hash[:32]> -iv 00000000<hash[32:44]> over zeros.
		want := unhex(t, "be814f5b3fd6afa792ae476ae0ce9f4716dd7d26df4e277574e107cb0a506c3df99b24bc34610c7a432412f28647b55c5c090c5951aeebc083e6daf8d8f45e8f09ed1be5fac6a3e203bb61623b620c6be369d516b15abda1e1acb422ff9fa68db394de12")
		s, err := NewInnerStream(InnerChaCha20, []byte(innerKey))
		if err != nil {
			t.Fatal(err)
		}
		got := make([]byte, len(want))
		s.XORKeyStream(got, got)
		if !bytes.Equal(got, want) {
			t.Errorf("keystream = %x, want %x", got, want)
		}
	})
	t.Run("Salsa20 against the library", func(t *testing.T) {
		key := sha256.Sum256([]byte(innerKey))
		iv := []byte{0xE8, 0x30, 0x09, 0x4B, 0x97, 0x20, 0x5D, 0x2A}
		want := make([]byte, 1000)
		salsa20.XORKeyStream(want, want, iv, &key)
		if !bytes.Equal(want[:8], []byte{want[0], want[1], want[2], want[3], want[4], want[5], want[6], want[7]}) || bytes.Count(want, []byte{0}) > 100 {
			t.Fatal("the reference keystream looks wrong")
		}
		for _, split := range [][]int{{1000}, {1, 999}, {63, 1, 64, 872}, {64, 64, 64, 808}, {65, 935}, {7, 7, 7, 979}, {500, 500}} {
			s, err := NewInnerStream(InnerSalsa20, []byte(innerKey))
			if err != nil {
				t.Fatal(err)
			}
			got := make([]byte, 1000)
			off := 0
			for _, n := range split {
				s.XORKeyStream(got[off:off+n], got[off:off+n])
				off += n
			}
			if !bytes.Equal(got, want) {
				t.Errorf("split %v gives another keystream", split)
			}
		}
	})
	t.Run("a stream encrypts and decrypts", func(t *testing.T) {
		for _, id := range []uint32{InnerSalsa20, InnerChaCha20} {
			enc, err := NewInnerStream(id, []byte("k"))
			if err != nil {
				t.Fatal(err)
			}
			dec, err := NewInnerStream(id, []byte("k"))
			if err != nil {
				t.Fatal(err)
			}
			msg := []byte("a password, then another one")
			ct := make([]byte, len(msg))
			enc.XORKeyStream(ct, msg)
			if bytes.Equal(ct, msg) {
				t.Errorf("stream %d does not change the data", id)
			}
			pt := make([]byte, len(ct))
			dec.XORKeyStream(pt, ct)
			if !bytes.Equal(pt, msg) {
				t.Errorf("stream %d does not decrypt: %q", id, pt)
			}
		}
	})
	t.Run("none leaves the data", func(t *testing.T) {
		s, err := NewInnerStream(InnerNone, nil)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]byte, 3)
		s.XORKeyStream(out, []byte("abc"))
		if string(out) != "abc" {
			t.Errorf("none changed the data: %q", out)
		}
	})
	t.Run("errors", func(t *testing.T) {
		_, err := NewInnerStream(InnerArcFour, []byte("k"))
		assertRoute(t, err, "NewInnerStream.InnerStreamUnsupported")
		_, err = NewInnerStream(99, []byte("k"))
		assertRoute(t, err, "NewInnerStream.InnerStreamUnsupported")
		for _, id := range []uint32{InnerSalsa20, InnerChaCha20, InnerArcFour} {
			_, err = NewInnerStream(id, nil)
			assertRoute(t, err, "NewInnerStream.InnerStreamKeyInvalid")
		}
	})
}

func TestErrorMessagesNameNoSecret(t *testing.T) {
	// The error of a wrong key carries sizes and names, never the key.
	aes, err := LookupCipher(CipherAES256)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("SECRET-KEY-0123456789-SECRET-KEY") // 32 bytes
	_, err = aes.Decrypt(key, make([]byte, 16), make([]byte, 5))
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("error = %v", err)
	}
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatal("not an *errors.Error")
	}
	for k, v := range appErr.Metadata {
		if strings.Contains(k, "key") && k != "keyLength" && k != "wantKeyLength" {
			t.Errorf("metadata %q = %v may hold a key", k, v)
		}
	}
}
