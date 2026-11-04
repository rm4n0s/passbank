package crypto

import (
	"context"
	"crypto/aes"
	"crypto/sha256"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/third_party/argon2"
)

// The UUIDs of the key-derivation functions of a KDBX file (SPEC §4.1).
var (
	// KDFAES is the AES-KDF: rounds of AES-256 over the key, then SHA-256.
	KDFAES = model.UUID{0xc9, 0xd9, 0xf3, 0x9a, 0x62, 0x8a, 0x44, 0x60, 0xbf, 0x74, 0x0d, 0x08, 0xc1, 0x8a, 0x4f, 0xea}
	// KDFAESKdbx4 is the other ID that KDBX 4 files carry for the AES-KDF: KeePassXC
	// writes it. It means the same function; the reader maps it to KDFAES, and
	// Passbank always writes KDFAES (SPEC §4.5).
	KDFAESKdbx4 = model.UUID{0x7c, 0x02, 0xbb, 0x82, 0x79, 0xa7, 0x4a, 0xc0, 0x92, 0x7d, 0x11, 0x4a, 0x00, 0x64, 0x82, 0x38}
	// KDFArgon2d is Argon2d, KeePassXC's default.
	KDFArgon2d = model.UUID{0xef, 0x63, 0x6d, 0xdf, 0x8c, 0x29, 0x44, 0x4b, 0x91, 0xf7, 0xa9, 0xa4, 0x03, 0xe3, 0x0a, 0x0c}
	// KDFArgon2id is Argon2id.
	KDFArgon2id = model.UUID{0x9e, 0x29, 0x8b, 0x19, 0x56, 0xdb, 0x47, 0x73, 0xb2, 0x3d, 0xfc, 0x3e, 0xc6, 0xf0, 0xa1, 0xe6}
)

// Argon2Version is the version of Argon2 that Passbank supports (1.3). Version
// 1.0 (0x10) is rejected: no program writes it by default and the library has
// only 1.3.
const Argon2Version = 0x13

// The bounds of the parameters of a key-derivation function. A file is not
// trusted: the parameters decide how much memory and time reading it takes, so
// they are checked before anything is allocated (SPEC §14).
const (
	// MaxAESRounds is the most rounds of the AES-KDF that Passbank runs.
	MaxAESRounds = 1<<32 - 1
	// MaxArgon2Memory is the most memory of Argon2 in bytes: 4 GiB.
	MaxArgon2Memory = 4 << 30
	// MaxArgon2Iterations is the most passes of Argon2.
	MaxArgon2Iterations = 1<<32 - 1
	// MaxArgon2Parallelism is the most lanes of Argon2 (the library takes a byte).
	MaxArgon2Parallelism = 255
	// MinArgon2SeedLength and MaxArgon2SeedLength bound the salt of Argon2.
	MinArgon2SeedLength = 8
	MaxArgon2SeedLength = 1 << 16
	// KeyLength is the length of the keys the KDFs take and give.
	KeyLength = 32
)

// aesRoundsPerCheck is how many rounds of the AES-KDF run between two looks at
// whether the context has ended.
const aesRoundsPerCheck = 1 << 16

// KDF holds a key-derivation function and its parameters, as a KDBX file states
// them.
type KDF struct {
	// ID is the UUID of the function: KDFAES, KDFArgon2d or KDFArgon2id.
	ID model.UUID
	// Seed is the transform seed of the AES-KDF (32 bytes) or the salt of Argon2.
	Seed []byte
	// Rounds is the number of rounds of the AES-KDF.
	Rounds uint64
	// Memory is the memory of Argon2 in bytes.
	Memory uint64
	// Iterations is the number of passes of Argon2.
	Iterations uint64
	// Parallelism is the number of lanes of Argon2.
	Parallelism uint32
	// Version is the version of Argon2; it must be Argon2Version.
	Version uint32
}

// Name returns the name of the function, such as "Argon2id".
func (k KDF) Name() string {
	switch k.ID {
	case KDFAES:
		return "AES-KDF"
	case KDFArgon2d:
		return "Argon2d"
	case KDFArgon2id:
		return "Argon2id"
	}
	return "unknown"
}

// Validate checks that the function is one Passbank supports (KdfUnsupported)
// and that its parameters are within bounds (KdfParametersRejected).
func (k KDF) Validate() error {
	switch k.ID {
	case KDFAES:
		if len(k.Seed) != KeyLength {
			return errors.New("KdfParametersRejected", "the seed of the AES-KDF must be 32 bytes", "kdf", k.Name(), "seedLength", len(k.Seed))
		}
		if k.Rounds < 1 || k.Rounds > MaxAESRounds {
			return errors.New("KdfParametersRejected", "the number of rounds of the AES-KDF is out of range",
				"kdf", k.Name(), "rounds", k.Rounds, "max", uint64(MaxAESRounds))
		}
	case KDFArgon2d, KDFArgon2id:
		if k.Version != Argon2Version {
			return errors.New("KdfParametersRejected", "Passbank supports only version 1.3 (0x13) of Argon2",
				"kdf", k.Name(), "version", k.Version)
		}
		if len(k.Seed) < MinArgon2SeedLength || len(k.Seed) > MaxArgon2SeedLength {
			return errors.New("KdfParametersRejected", "the salt of Argon2 has an unsupported length",
				"kdf", k.Name(), "saltLength", len(k.Seed), "min", MinArgon2SeedLength, "max", MaxArgon2SeedLength)
		}
		if k.Parallelism < 1 || k.Parallelism > MaxArgon2Parallelism {
			return errors.New("KdfParametersRejected", "the parallelism of Argon2 is out of range",
				"kdf", k.Name(), "parallelism", k.Parallelism, "max", MaxArgon2Parallelism)
		}
		if k.Iterations < 1 || k.Iterations > MaxArgon2Iterations {
			return errors.New("KdfParametersRejected", "the number of iterations of Argon2 is out of range",
				"kdf", k.Name(), "iterations", k.Iterations, "max", uint64(MaxArgon2Iterations))
		}
		// The library counts memory in KiB and needs at least 8 KiB per lane.
		if k.Memory > MaxArgon2Memory || k.Memory/1024 < 8*uint64(k.Parallelism) {
			return errors.New("KdfParametersRejected", "the memory of Argon2 is out of range",
				"kdf", k.Name(), "memory", k.Memory, "min", 8*1024*uint64(k.Parallelism), "max", uint64(MaxArgon2Memory))
		}
	default:
		return errors.New("KdfUnsupported", "the file uses a key-derivation function that Passbank does not support", "kdf", k.ID.String())
	}
	return nil
}

// Derive turns the 32-byte composite key into the 32-byte transformed key. It
// stops with KdfCancelled when ctx ends: the AES-KDF notices within a fraction of
// a second, and Argon2, which cannot be interrupted, is abandoned (it finishes in
// the background and its result is dropped).
func (k KDF) Derive(ctx context.Context, key []byte) ([]byte, error) {
	if err := k.Validate(); err != nil {
		return nil, err
	}
	if len(key) != KeyLength {
		return nil, errors.New("KdfKeyInvalid", "the key to transform must be 32 bytes", "keyLength", len(key))
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.NewErr("KdfCancelled", err)
	}
	if k.ID == KDFAES {
		return k.deriveAES(ctx, key)
	}
	return k.deriveArgon2(ctx, key)
}

// deriveAES runs the rounds of the AES-KDF: each 16-byte half of the key is
// encrypted Rounds times with AES-256 in ECB mode, the seed being the AES key;
// the transformed key is the SHA-256 of the two halves.
func (k KDF) deriveAES(ctx context.Context, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.Seed)
	if err != nil {
		return nil, errors.NewErr("CipherInitFailed", err).SetMetadata("cipher", "AES-KDF")
	}
	work := append([]byte(nil), key...)
	defer clear(work)
	lo, hi := work[:16], work[16:]
	for done := uint64(0); done < k.Rounds; {
		if err := ctx.Err(); err != nil {
			return nil, errors.NewErr("KdfCancelled", err)
		}
		n := min(k.Rounds-done, aesRoundsPerCheck)
		for range n {
			block.Encrypt(lo, lo)
			block.Encrypt(hi, hi)
		}
		done += n
	}
	sum := sha256.Sum256(work)
	return sum[:], nil
}

// deriveArgon2 runs Argon2d or Argon2id with the composite key as the password.
func (k KDF) deriveArgon2(ctx context.Context, key []byte) ([]byte, error) {
	run := argon2.IDKey
	if k.ID == KDFArgon2d {
		run = argon2.DKey
	}
	password := append([]byte(nil), key...)
	done := make(chan []byte, 1)
	go func() {
		defer clear(password)
		done <- run(password, k.Seed, uint32(k.Iterations), uint32(k.Memory/1024), uint8(k.Parallelism), KeyLength)
	}()
	select {
	case out := <-done:
		return out, nil
	case <-ctx.Done():
		return nil, errors.NewErr("KdfCancelled", ctx.Err())
	}
}
