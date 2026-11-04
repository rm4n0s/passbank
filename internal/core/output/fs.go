package output

import (
	"io"
	"io/fs"
	"os"
)

// FS is the part of the file system that Write uses, so that tests can make each
// step fail.
type FS interface {
	// Stat follows links, as os.Stat does.
	Stat(name string) (fs.FileInfo, error)
	// CreateTemp makes a new file with mode 0600 in dir, as os.CreateTemp does.
	CreateTemp(dir, pattern string) (File, error)
	// Link makes newname another name of oldname and fails if newname exists.
	Link(oldname, newname string) error
	// Rename moves oldname to newname, replacing it.
	Rename(oldname, newname string) error
	// Remove deletes a file.
	Remove(name string) error
	// SyncDir flushes the entries of a folder to disk.
	SyncDir(dir string) error
}

// File is a file being written.
type File interface {
	io.Writer
	// Name is the path of the file.
	Name() string
	// Sync flushes the content to disk.
	Sync() error
	// Close closes the file.
	Close() error
}

// OS is the real file system.
type OS struct{}

// Stat is os.Stat.
func (OS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }

// CreateTemp is os.CreateTemp.
func (OS) CreateTemp(dir, pattern string) (File, error) { return os.CreateTemp(dir, pattern) }

// Link is os.Link.
func (OS) Link(oldname, newname string) error { return os.Link(oldname, newname) }

// Rename is os.Rename.
func (OS) Rename(oldname, newname string) error { return os.Rename(oldname, newname) }

// Remove is os.Remove.
func (OS) Remove(name string) error { return os.Remove(name) }

// SyncDir opens the folder and syncs it.
func (OS) SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close() // the sync error is the one to report
		return err
	}
	return d.Close()
}
