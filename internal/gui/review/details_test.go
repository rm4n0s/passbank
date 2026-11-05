package review

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/gui/kit"
)

// row returns the row with the name, or fails.
func row(t *testing.T, d Details, name string) kit.FieldRow {
	t.Helper()
	for _, r := range d.Rows {
		if r.Name == name {
			return r
		}
	}
	names := make([]string, len(d.Rows))
	for i, r := range d.Rows {
		names[i] = r.Name
	}
	t.Fatalf("no row %q among %v", name, names)
	return kit.FieldRow{}
}

func hasRow(d Details, name string) bool {
	for _, r := range d.Rows {
		if r.Name == name {
			return true
		}
	}
	return false
}

func TestDetailsOfAPairThatDiffers(t *testing.T) {
	a, b := modeltest.MergePair()
	changes := diff.Changes(a, b)
	bank := DetailsOf(changes[3], a, b) // Banking / My bank
	tests := []struct {
		row       string
		a, b      string
		differs   bool
		protected bool
	}{
		{"Title", "My bank", "My bank", false, false},
		{"User name", "me", "me", false, false},
		{"Password", "bank-password-a", "bank-password-b", true, true},
		{"URL", "https://bank.example.com", "https://bank.example.org", true, false},
		{"Group", "Banking", "Banking", false, false},
		{"Modified", "1970-01-01", "1970-01-01", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.row, func(t *testing.T) {
			r := row(t, bank, tt.row)
			if tt.row == "Modified" {
				if r.A == r.B || r.A == "" || r.B == "" || r.Differs {
					t.Errorf("both modification times must show and differ in text: %q, %q", r.A, r.B)
				}
				return
			}
			if r.A != tt.a || r.B != tt.b || r.Differs != tt.differs || r.Protected != tt.protected {
				t.Errorf("row = %+v", r)
			}
		})
	}
	if len(bank.Also) != 0 {
		t.Errorf("Also = %v; every difference has a row", bank.Also)
	}
	store := DetailsOf(changes[4], a, b)
	if r := row(t, store, "Title"); r.A != "Store" || r.B != "Store (old name)" || !r.Differs {
		t.Errorf("title row = %+v", r)
	}
	if r := row(t, store, "Password"); r.Differs || !r.Protected {
		t.Errorf("an equal password must not be marked: %+v", r)
	}
}

func TestDetailsOfOneSidedChanges(t *testing.T) {
	a, b := modeltest.MergePair()
	changes := diff.Changes(a, b)
	tests := []struct {
		name         string
		index        int
		missingA     bool
		missingB     bool
		deletedRow   bool
		wantTitleA   string
		wantTitleB   string
		modifiedBoth bool
	}{
		{"only in A", 0, false, true, false, "Work mail", "", false},
		{"only in B", 2, true, false, false, "", "Old forum", false},
		{"deleted in A", 5, true, false, true, "", "Old account", false},
		{"deleted in B", 6, false, true, true, "Stale login", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := DetailsOf(changes[tt.index], a, b)
			title := row(t, d, "Title")
			if title.A != tt.wantTitleA || title.B != tt.wantTitleB || title.MissingA != tt.missingA || title.MissingB != tt.missingB {
				t.Errorf("title row = %+v", title)
			}
			if title.Differs {
				t.Error("a side with no entry is not a difference in a field")
			}
			if hasRow(d, "Deleted") != tt.deletedRow {
				t.Errorf("Deleted row present = %v, want %v", hasRow(d, "Deleted"), tt.deletedRow)
			}
			m := row(t, d, "Modified")
			if m.MissingA != tt.missingA || m.MissingB != tt.missingB {
				t.Errorf("modified row = %+v", m)
			}
		})
	}
	// The deletion time shows on the side that deleted.
	d := DetailsOf(changes[5], a, b)
	if r := row(t, d, "Deleted"); r.A == "" || !r.MissingB {
		t.Errorf("Deleted row of deleted-in-a = %+v", r)
	}
	d = DetailsOf(changes[6], a, b)
	if r := row(t, d, "Deleted"); r.B == "" || !r.MissingA {
		t.Errorf("Deleted row of deleted-in-b = %+v", r)
	}
}

func TestDetailsOfGroupsAndSettings(t *testing.T) {
	a, b := modeltest.MergePair()
	changes := diff.Changes(a, b)

	g := DetailsOf(changes[7], a, b) // Shopping (group): the notes differ
	if r := row(t, g, "Notes"); r.A != "" || r.B != "Ship to the office" || !r.Differs {
		t.Errorf("notes row = %+v", r)
	}
	if r := row(t, g, "Name"); r.A != "Shopping" || r.Differs {
		t.Errorf("name row = %+v", r)
	}
	onlyB := DetailsOf(changes[1], a, b) // Games (group)
	if r := row(t, onlyB, "Name"); !r.MissingA || r.B != "Games" {
		t.Errorf("name row of a group only B has = %+v", r)
	}

	s := DetailsOf(changes[8], a, b)
	if r := row(t, s, "Database name"); r.A != "Personal" || r.B != "Laptop" || !r.Differs {
		t.Errorf("database name row = %+v", r)
	}
	if r := row(t, s, "Description"); r.Differs {
		t.Errorf("description row = %+v", r)
	}
}

// pairWithAttachments is the merge fixtures with attachments on the entry "My bank":
// statement.pdf is the same on both sides, notes.txt differs, key.txt only A has.
func pairWithAttachments(t *testing.T) (a, b *model.Database, changes []diff.Change) {
	t.Helper()
	a, b = modeltest.MergePair()
	attach := func(db *model.Database, name, content string) model.Attachment {
		sum := sha256.Sum256([]byte(content))
		key := model.BinaryKey{Hash: sum}
		db.Binaries[key] = &model.Binary{Data: secret.New([]byte(content))}
		return model.Attachment{Name: name, Binary: key}
	}
	find := func(db *model.Database) *model.Entry {
		for _, g := range db.Root.Groups {
			for _, e := range g.Entries {
				if e.Title() == "My bank" {
					return e
				}
			}
		}
		t.Fatal("no My bank")
		return nil
	}
	ea, eb := find(a), find(b)
	ea.Attachments = []model.Attachment{attach(a, "statement.pdf", "same"), attach(a, "notes.txt", "one"), attach(a, "key.txt", "k")}
	eb.Attachments = []model.Attachment{attach(b, "statement.pdf", "same"), attach(b, "notes.txt", "two!")}
	return a, b, diff.Changes(a, b)
}

func TestDetailsOfAttachments(t *testing.T) {
	a, b, changes := pairWithAttachments(t)
	d := DetailsOf(changes[3], a, b)
	tests := []struct {
		row      string
		a, b     string
		differs  bool
		missingB bool
	}{
		{"Attachment (same)", "statement.pdf (4 bytes)", "statement.pdf (4 bytes)", false, false},
		{"Attachment (different)", "", "", true, false}, // two rows have this name; checked below
	}
	same := row(t, d, tests[0].row)
	if same.A != tests[0].a || same.B != tests[0].b || same.Differs {
		t.Errorf("same attachment = %+v", same)
	}
	var different []kit.FieldRow
	for _, r := range d.Rows {
		if r.Name == "Attachment (different)" {
			different = append(different, r)
		}
	}
	if len(different) != 2 {
		t.Fatalf("%d different attachments, want notes.txt and key.txt", len(different))
	}
	for _, r := range different {
		if !r.Differs {
			t.Errorf("a different attachment is not marked: %+v", r)
		}
	}
	if different[0].A != "notes.txt (3 bytes)" || different[0].B != "notes.txt (4 bytes)" {
		t.Errorf("notes.txt row = %+v", different[0])
	}
	if different[1].A != "key.txt (1 byte)" || !different[1].MissingB {
		t.Errorf("key.txt row = %+v", different[1])
	}
	if len(d.Also) != 0 {
		t.Errorf("Also = %v", d.Also)
	}
}

func TestDetailsNeverPutAProtectedValueInALabel(t *testing.T) {
	a, b := modeltest.MergePair()
	for i, c := range diff.Changes(a, b) {
		d := DetailsOf(c, a, b)
		for _, r := range d.Rows {
			if r.Name == "Password" && !r.Protected {
				t.Errorf("change %d: the password row is not protected", i)
			}
			if strings.Contains(r.Name, "password") {
				t.Errorf("change %d: row name %q holds a value", i, r.Name)
			}
		}
	}
}

func TestDetailsAlsoNamesWhatHasNoRow(t *testing.T) {
	a, b := modeltest.MergePair()
	// A different auto-type setting has no row of its own.
	for _, db := range []*model.Database{a, b} {
		for _, g := range db.Root.Groups {
			for _, e := range g.Entries {
				if e.Title() == "My bank" {
					e.AutoType.Enabled = db == a
				}
			}
		}
	}
	d := DetailsOf(diff.Changes(a, b)[3], a, b)
	if len(d.Also) != 1 || d.Also[0] != diff.FieldAutoType {
		t.Errorf("Also = %v, want [auto-type]", d.Also)
	}
}
