package kdbx

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// signature is the start of every KDBX file: the two signatures that KeePass 2 files carry.
var signature = []byte{0x03, 0xd9, 0xa2, 0x9a, 0x67, 0xfb, 0x4b, 0xb5}

// TestFixturesAreListed keeps testdata/ and testdata/FIXTURES.md in step: every database and key file of
// the folder is in the list, every file the list names is in the folder, and every database starts with
// the signature and the version that its name and the list say.
func TestFixturesAreListed(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	var onDisk []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".kdbx") || strings.HasPrefix(name, "keyfile-") {
			onDisk = append(onDisk, name)
		}
	}
	list, err := os.ReadFile(filepath.Join("testdata", "FIXTURES.md"))
	if err != nil {
		t.Fatal(err)
	}
	// A file is listed when its name appears in backticks at the start of a table row.
	var listed []string
	for _, line := range strings.Split(string(list), "\n") {
		if rest, ok := strings.CutPrefix(line, "| `"); ok {
			if name, _, ok := strings.Cut(rest, "`"); ok {
				listed = append(listed, name)
			}
		}
	}
	slices.Sort(onDisk)
	slices.Sort(listed)
	if !slices.Equal(onDisk, listed) {
		t.Errorf("testdata and FIXTURES.md disagree:\n on disk: %q\n listed:  %q", onDisk, listed)
	}

	wantVersion := map[string][2]uint16{ // by name prefix: major, minor
		"k31-": {3, 1}, "k41-": {4, 1}, "k4-": {4, 0},
	}
	for _, name := range onDisk {
		if !strings.HasSuffix(name, ".kdbx") {
			continue
		}
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) < 12 || !slices.Equal(data[:8], signature) {
			t.Errorf("%s does not start with the KDBX signature", name)
			continue
		}
		minor, major := binary.LittleEndian.Uint16(data[8:10]), binary.LittleEndian.Uint16(data[10:12])
		for prefix, want := range wantVersion {
			// k4- must not match k41- (the longer prefix k41- is a different key).
			if strings.HasPrefix(name, prefix) && (prefix != "k4-" || !strings.HasPrefix(name, "k41-")) {
				if major != want[0] || minor != want[1] {
					t.Errorf("%s is version %d.%d, its name says %d.%d", name, major, minor, want[0], want[1])
				}
			}
		}
	}
}
