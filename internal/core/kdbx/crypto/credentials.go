package crypto

import (
	"context"
	"crypto/sha256"
	"io"
	"os"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/secret"
)

// MaxKeyFileSize is the largest key file Passbank reads: 64 MiB.
const MaxKeyFileSize = 64 << 20

// Credentials are what unlocks a database: a password, a key file, or both
// (SPEC §4.2).
type Credentials struct {
	// HasPassword says that the database has a password component. The password may
	// then be empty: KeePass treats an empty password as a component, and so does
	// this field. A caller that wants "no password" (the CLI's --no-password, or an
	// empty password field, as KeePassXC reads it) leaves HasPassword false.
	HasPassword bool
	// Password is what the user typed, as UTF-8 bytes, without normalization.
	Password secret.Bytes
	// HasKeyFile says that the database has a key file component.
	HasKeyFile bool
	// KeyFile is the content of the key file.
	KeyFile secret.Bytes
}

// Wipe overwrites the password and the key file content.
func (c Credentials) Wipe() {
	c.Password.Wipe()
	c.KeyFile.Wipe()
}

// CompositeKey makes the 32-byte composite key of the credentials, as KeePass
// does: the SHA-256 of the password and the key of the key file, in that order,
// each as its own 32 bytes (the password is hashed first). It is
// NoCredentials when there is neither a password nor a key file, and
// KeyFileCorrupt when the key file is an XML key file that is damaged.
func CompositeKey(c Credentials) (secret.Bytes, error) {
	if !c.HasPassword && !c.HasKeyFile {
		return secret.Bytes{}, errors.New("NoCredentials", "a database needs a password, a key file, or both")
	}
	var parts []byte
	defer func() { clear(parts) }()
	if c.HasPassword {
		sum := sha256.Sum256(c.Password.Reveal())
		parts = append(parts, sum[:]...)
		clear(sum[:])
	}
	if c.HasKeyFile {
		key, err := keyFileKey(c.KeyFile.Reveal())
		if err != nil {
			return secret.Bytes{}, err
		}
		parts = append(parts, key...)
		clear(key)
	}
	sum := sha256.Sum256(parts)
	return secret.New(sum[:]), nil
}

// ReadKeyFile reads a key file from disk, at most MaxKeyFileSize bytes. The
// content is a secret: wipe it when it is no longer needed.
func ReadKeyFile(ctx context.Context, path string) (secret.Bytes, error) {
	if err := ctx.Err(); err != nil {
		return secret.Bytes{}, errors.NewErr("KeyFileReadCancelled", err).SetMetadata("path", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return secret.Bytes{}, errors.NewErr("KeyFileReadFailed", err).SetMetadata("path", path)
	}
	defer func() { _ = f.Close() }() // read only: a failed close loses nothing
	data, err := io.ReadAll(io.LimitReader(f, MaxKeyFileSize+1))
	if err != nil {
		clear(data)
		return secret.Bytes{}, errors.NewErr("KeyFileReadFailed", err).SetMetadata("path", path)
	}
	if len(data) > MaxKeyFileSize {
		clear(data)
		return secret.Bytes{}, errors.New("KeyFileTooLarge", "the key file is larger than 64 MiB", "path", path, "max", MaxKeyFileSize)
	}
	return secret.New(data), nil
}
