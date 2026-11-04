package kdbx

import (
	"context"
	"io"
	"os"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
)

// MaxFileSize is the largest file Open reads: 2 GiB.
const MaxFileSize = 2 << 30

// Info describes a database file: how it is stored and what it holds. It holds
// no secret.
type Info struct {
	// Version is the KDBX version of the file.
	Version Version
	// CipherID and Cipher are the outer cipher.
	CipherID model.UUID
	Cipher   string
	// KDF is the key-derivation function with its parameters.
	KDF crypto.KDF
	// Compression is how the payload is compressed.
	Compression Compression
	// InnerStreamID and InnerStream say how protected values are encrypted.
	InnerStreamID uint32
	InnerStream   string
	// Groups counts all groups, the root included; Entries all entries (those in
	// the recycle bin too, history not); HistoryItems the history versions of all
	// entries; Attachments the attachments of all entries (history not).
	Groups, Entries, HistoryItems, Attachments int
}

// InnerStreamName returns the name of an inner stream, such as "ChaCha20".
func InnerStreamName(id uint32) string {
	switch id {
	case crypto.InnerNone:
		return "none"
	case crypto.InnerArcFour:
		return "ArcFourVariant"
	case crypto.InnerSalsa20:
		return "Salsa20"
	case crypto.InnerChaCha20:
		return "ChaCha20"
	}
	return "unknown"
}

// Open reads the database at path with the credentials. The file is never
// changed. The result holds secrets (protected values, attachments); the caller
// wipes it with model.Database.Wipe when it is done.
func Open(ctx context.Context, path string, creds crypto.Credentials) (*model.Database, Info, error) {
	data, err := readDatabaseFile(ctx, path, MaxFileSize)
	if err != nil {
		return nil, Info{}, err
	}
	return OpenBytes(ctx, data, creds)
}

// readDatabaseFile reads a database file of at most max bytes.
func readDatabaseFile(ctx context.Context, path string, max int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.NewErr("ReadCancelled", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.NewErr("FileReadFailed", err).SetMetadata("path", path)
	}
	defer f.Close() //nolint:errcheck // the file was only read
	if fi, err := f.Stat(); err == nil && fi.Size() > max {
		return nil, errors.New("FileTooLarge", "the file is larger than Passbank reads", "path", path, "limit", max)
	}
	// The read is bounded too, for a file that grows or has no size.
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, errors.NewErr("FileReadFailed", err).SetMetadata("path", path)
	}
	if int64(len(data)) > max {
		return nil, errors.New("FileTooLarge", "the file is larger than Passbank reads", "path", path, "limit", max)
	}
	return data, nil
}

// OpenBytes is Open for a file that is already in memory.
func OpenBytes(ctx context.Context, data []byte, creds crypto.Credentials) (*model.Database, Info, error) {
	version, err := parseSignature(data)
	if err != nil {
		return nil, Info{}, err
	}
	if version.Major < 2 || version.Major > 4 {
		return nil, Info{}, errors.New("VersionUnsupported", "the file uses a KDBX version that Passbank does not know", "version", version.String())
	}
	composite, err := crypto.CompositeKey(creds)
	if err != nil {
		return nil, Info{}, err
	}
	defer composite.Wipe()
	read := readContainer4
	if version.Major < 4 {
		read = readContainer3
	}
	c, err := read(ctx, data, composite)
	if err != nil {
		return nil, Info{}, err
	}
	defer c.wipe()
	// The key derivation is the slow part; look at ctx before the parsing.
	if err := ctx.Err(); err != nil {
		return nil, Info{}, errors.NewErr("ReadCancelled", err)
	}
	db, err := parseXML(ctx, c)
	if err != nil {
		return nil, Info{}, err
	}
	cipher, err := crypto.LookupCipher(c.Header.CipherID)
	if err != nil {
		db.Wipe()
		return nil, Info{}, err
	}
	info := Info{
		Version: c.Header.Version, CipherID: cipher.ID(), Cipher: cipher.Name(), KDF: c.Header.KDF,
		Compression: c.Header.Compression, InnerStreamID: c.InnerStreamID, InnerStream: InnerStreamName(c.InnerStreamID),
	}
	info.Groups, info.Entries, info.HistoryItems, info.Attachments = count(db)
	return db, info, nil
}

// count counts the groups, entries, history versions and attachments of a database.
func count(db *model.Database) (groups, entries, history, attachments int) {
	db.WalkGroups(func(*model.Group) bool { groups++; return true })
	db.WalkEntries(func(_ *model.Group, e *model.Entry) bool {
		entries++
		history += len(e.History)
		attachments += len(e.Attachments)
		return true
	})
	return
}
