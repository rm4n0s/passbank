package output

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
)

// Options of Write.
type Options struct {
	// Overwrite lets Write replace a file that exists at the path. Without it an
	// existing file is OutputExists, and it is never replaced, not even by a race.
	Overwrite bool
	// Inputs are the paths of the databases that were merged. The output may not be
	// any of them (OutputIsInput), also with Overwrite.
	Inputs []string
	// FS is the file system; nil is the real one.
	FS FS
}

// maxDifferences is how many lines of the differences a WriteVerifyFailed carries.
const maxDifferences = 20

// Write writes the database to path as a file of the version, encrypted with the
// settings and the credentials, and verifies it (SPEC §5):
//
//   - the content goes to a temporary file in the folder of path (mode 0600), is
//     synced, and read back with the same credentials; it must equal the database
//     as the version holds it (kdbx.Fit), or the write is WriteVerifyFailed
//   - only then is it put in place, by a link that fails if the path exists (or by
//     a rename when Overwrite is set), and the folder is synced
//   - a failure leaves no temporary file and leaves a file that was at path as it
//     was. The one exception is a failure to sync the folder after the file is in
//     place: the file is complete, but may not survive a crash
//
// The database and the credentials are not changed.
func Write(ctx context.Context, path string, db *model.Database, version kdbx.Version, settings kdbx.EncryptionSettings, creds crypto.Credentials, opts Options) error {
	return write(ctx, path, db, version, settings, creds, opts, kdbx.Fit(db, version))
}

// write is Write with the database that the file must read back as, so that a
// test can make the comparison fail.
func write(ctx context.Context, path string, db *model.Database, version kdbx.Version, settings kdbx.EncryptionSettings, creds crypto.Credentials, opts Options, expected *model.Database) (err error) {
	fsys := opts.FS
	if fsys == nil {
		fsys = OS{}
	}
	if path == "" {
		return errors.New("OutputPathInvalid", "the output path is empty")
	}
	if err := checkInputs(fsys, path, opts.Inputs); err != nil {
		return err
	}
	if _, statErr := fsys.Stat(path); statErr == nil && !opts.Overwrite {
		return errors.New("OutputExists", "the output file exists; ask to replace it", "path", path)
	}

	data, err := kdbx.Encode(ctx, db, version, settings, creds)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := fsys.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return errors.NewErr("OutputTempFailed", err).SetMetadata("dir", dir)
	}
	tmpPath := tmp.Name()
	placed := false
	defer func() {
		if !placed {
			if rmErr := fsys.Remove(tmpPath); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) && err != nil {
				if appErr, ok := errors.FromError(err); ok {
					_ = appErr.SetMetadata("tempFileLeft", tmpPath) // it changes appErr in place
				}
			}
		}
	}()
	if _, werr := tmp.Write(data); werr != nil {
		_ = tmp.Close() // the write error is the one to report
		return errors.NewErr("OutputWriteFailed", werr).SetMetadata("path", tmpPath)
	}
	if serr := tmp.Sync(); serr != nil {
		_ = tmp.Close() // the sync error is the one to report
		return errors.NewErr("OutputWriteFailed", serr).SetMetadata("path", tmpPath)
	}
	if cerr := tmp.Close(); cerr != nil {
		return errors.NewErr("OutputWriteFailed", cerr).SetMetadata("path", tmpPath)
	}

	if err := verify(ctx, tmpPath, creds, expected); err != nil {
		return err
	}

	if opts.Overwrite {
		if rerr := fsys.Rename(tmpPath, path); rerr != nil {
			return errors.NewErr("OutputPlaceFailed", rerr).SetMetadata("path", path)
		}
		placed = true
	} else {
		if lerr := fsys.Link(tmpPath, path); lerr != nil {
			if errors.Is(lerr, fs.ErrExist) {
				return errors.New("OutputExists", "the output file exists; ask to replace it", "path", path)
			}
			return errors.NewErr("OutputPlaceFailed", lerr).SetMetadata("path", path)
		}
		placed = true
		if rmErr := fsys.Remove(tmpPath); rmErr != nil {
			return errors.NewErr("OutputPlaceFailed", rmErr).SetMetadata("path", tmpPath)
		}
	}
	if serr := fsys.SyncDir(dir); serr != nil {
		return errors.NewErr("OutputSyncFailed", serr).SetMetadata("dir", dir)
	}
	return nil
}

// checkInputs refuses a path that is one of the inputs: the same file, also
// through a link or a different spelling of the path.
func checkInputs(fsys FS, path string, inputs []string) error {
	out, err := fsys.Stat(path)
	if err != nil {
		return nil // a file that does not exist is nobody's input
	}
	for _, in := range inputs {
		info, err := fsys.Stat(in)
		if err == nil && os.SameFile(out, info) {
			return errors.New("OutputIsInput", "the output must be a new file, not one of the inputs", "path", path, "input", in)
		}
	}
	return nil
}

// verify opens the file that was written with the credentials and requires the
// database to equal expected.
func verify(ctx context.Context, path string, creds crypto.Credentials, expected *model.Database) error {
	got, _, err := kdbx.Open(ctx, path, creds)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return errors.NewErr("WriteCancelled", cerr)
		}
		cause := err.Error()
		if appErr, ok := errors.FromError(err); ok {
			cause = appErr.Tag
		}
		return errors.New("WriteVerifyFailed", "the written file cannot be read back", "cause", cause)
	}
	defer got.Wipe()
	if diff := model.Diff(expected, got); len(diff) != 0 {
		shown := diff[:min(len(diff), maxDifferences)]
		return errors.New("WriteVerifyFailed", "the written file reads back differently", "differences", strings.Join(shown, "; "), "count", len(diff))
	}
	return nil
}
