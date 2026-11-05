package plan

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/diff"
)

// maxFile is the largest plan file ReadFile reads.
const maxFile = 256 << 20

// ReadFile reads the plan file at path with Read, for the inputs a and b and the
// changes between them. It also fails with PlanFileReadFailed when the file cannot
// be opened and with ReadCancelled when ctx has ended.
func ReadFile(ctx context.Context, path string, a, b Input, changes []diff.Change) (*Plan, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.NewErr("ReadCancelled", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.NewErr("PlanFileReadFailed", err).SetMetadata("path", path)
	}
	defer func() { _ = f.Close() }() // the file was only read
	return Read(io.LimitReader(f, maxFile), a, b, changes)
}

// WriteFile writes the plan text to path. A file that exists is never replaced
// unless replace is set (PlanFileExists); then the new file is written beside it
// and renamed over it, so that a failure leaves the old one whole. The file is
// readable only by its owner, because titles can be sensitive.
func WriteFile(ctx context.Context, path string, data []byte, replace bool) error {
	if err := ctx.Err(); err != nil {
		return errors.NewErr("WriteCancelled", err)
	}
	var err error
	if replace {
		err = replaceFile(path, data)
	} else {
		err = createFile(path, data)
	}
	if err == nil {
		return nil
	}
	if !replace && errors.Is(err, fs.ErrExist) {
		return errors.New("PlanFileExists", "the plan file exists", "path", path)
	}
	return errors.NewErr("PlanFileWriteFailed", err).SetMetadata("path", path)
}

// CheckPath refuses a plan file that is one of the databases: writing it would
// destroy that database (PlanOutputIsInput). A path that does not exist yet counts
// by its folder and name, and a link to a database counts as the database.
func CheckPath(path string, databases ...string) error {
	same, err := samePath(path, databases...)
	if err != nil {
		return err
	}
	if same {
		return errors.New("PlanOutputIsInput", "the plan file must not be one of the databases", "path", path)
	}
	return nil
}

// samePath reports whether path is the same file as any of others (also when it
// does not exist yet, by its folder and name).
func samePath(path string, others ...string) (bool, error) {
	pa, err := filepath.Abs(path)
	if err != nil {
		return false, errors.NewErr("OutputPathInvalid", err).SetMetadata("path", path)
	}
	for _, o := range others {
		oa, err := filepath.Abs(o)
		if err != nil {
			return false, errors.NewErr("OutputPathInvalid", err).SetMetadata("path", o)
		}
		if pa == oa {
			return true, nil
		}
		if fi, err := os.Stat(path); err == nil {
			if fo, err := os.Stat(o); err == nil && os.SameFile(fi, fo) {
				return true, nil
			}
		}
	}
	return false, nil
}

// createFile writes a new file that must not exist, and removes it again if the
// write fails.
func createFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path) // the file is ours and incomplete
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

// replaceFile writes the data to a temporary file in the folder of path and
// renames it over path.
func replaceFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".passbank-plan-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}
