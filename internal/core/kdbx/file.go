package kdbx

import (
	"encoding/binary"
	"strconv"

	"github.com/rm4n0s/errors"
)

// The signatures at the start of every KeePass file (SPEC §4.1).
const (
	// signature1 is the first signature of KeePass 1 and KeePass 2 files.
	signature1 = 0x9AA2D903
	// signature2KDBX is the second signature of KDBX files (KeePass 2 and later).
	signature2KDBX = 0xB54BFB67
	// signature2KDB is the second signature of KeePass 1 files (.kdb).
	signature2KDB = 0xB54BFB65
	// signature2Beta is the second signature of the pre-release KeePass 2 format.
	signature2Beta = 0xB54BFB66
)

// signatureSize is the size of the two signatures and the version: 12 bytes.
const signatureSize = 12

// Version is the version of a KDBX file: the major number says the container
// (2 and 3, or 4), the minor number the elements that are allowed in it.
type Version struct {
	Major, Minor uint16
}

// String formats the version as "4.1".
func (v Version) String() string {
	return strconv.Itoa(int(v.Major)) + "." + strconv.Itoa(int(v.Minor))
}

// parseSignature checks the two signatures and returns the version. It is
// NotKdbx for data that is not a KeePass file, KeePass1NotSupported for a .kdb
// file, and VersionUnsupported for the pre-release format.
func parseSignature(data []byte) (Version, error) {
	if len(data) < signatureSize {
		return Version{}, errors.New("NotKdbx", "the file is too short to be a KeePass database", "length", len(data))
	}
	if binary.LittleEndian.Uint32(data[0:4]) != signature1 {
		return Version{}, errors.New("NotKdbx", "the file is not a KeePass database")
	}
	switch binary.LittleEndian.Uint32(data[4:8]) {
	case signature2KDBX:
	case signature2KDB:
		return Version{}, errors.New("KeePass1NotSupported", "the file is a KeePass 1 database (.kdb)")
	case signature2Beta:
		return Version{}, errors.New("VersionUnsupported", "the file is in the pre-release format of KeePass 2", "version", "beta")
	default:
		return Version{}, errors.New("NotKdbx", "the file is not a KeePass database")
	}
	return Version{
		Minor: binary.LittleEndian.Uint16(data[8:10]),
		Major: binary.LittleEndian.Uint16(data[10:12]),
	}, nil
}
