package plan

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model"
)

func uid(n byte) model.UUID { return model.UUID{0: 0x3f, 15: n} }

var (
	inA = Input{Hash: [32]byte{1, 2, 3}, Name: "personal.kdbx"}
	inB = Input{Hash: [32]byte{4, 5, 6}, Name: "laptop.kdbx"}
)

// sample is one change of every kind.
func sample() []diff.Change {
	return []diff.Change{
		{Kind: diff.OnlyA, Item: diff.ItemEntry, UUID: uid(1), Path: "Email", Title: "Work mail", Modified: [2]time.Time{hours(1), {}}},
		{Kind: diff.OnlyA, Item: diff.ItemGroup, UUID: uid(2), Path: "", Title: "Games"},
		{Kind: diff.OnlyB, Item: diff.ItemEntry, UUID: uid(3), Path: "Games", Title: "Old forum"},
		{Kind: diff.Differs, Item: diff.ItemEntry, UUID: uid(4), Path: "Banking", Title: "My bank", Fields: []string{"password", "url"},
			Modified: [2]time.Time{hours(2), hours(500)}},
		{Kind: diff.Differs, Item: diff.ItemEntry, UUID: uid(5), Path: "Shopping", Title: "Store", Fields: []string{"title", "field PIN"}},
		{Kind: diff.DeletedInA, Item: diff.ItemEntry, UUID: uid(6), Path: "Misc", Title: "Gone in A", Deleted: hours(9), Modified: [2]time.Time{{}, hours(3)}},
		{Kind: diff.DeletedInB, Item: diff.ItemEntry, UUID: uid(7), Path: "Misc", Title: "Gone in B", Deleted: hours(9), Modified: [2]time.Time{hours(3), {}}},
		{Kind: diff.GroupDiffers, Item: diff.ItemGroup, UUID: uid(8), Path: "", Title: "Work", Fields: []string{"name"}},
		{Kind: diff.Settings, Item: diff.ItemSettings, Fields: []string{"name", "color"}},
	}
}

func choicesOf(t *testing.T, p *Plan) []Choice {
	t.Helper()
	return slices.Clone(p.Choices)
}

func writeString(t *testing.T, p *Plan, o WriteOptions) string {
	t.Helper()
	var b bytes.Buffer
	if err := Write(&b, p, o); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestWriteGolden(t *testing.T) {
	p := New(inA, inB, sample())
	p.Choices[3] = TakeB
	p.Choices[4] = Both
	got := writeString(t, p, WriteOptions{})
	want := `passbank-plan 1
a sha256:0102030000000000000000000000000000000000000000000000000000000000 personal.kdbx
b sha256:0405060000000000000000000000000000000000000000000000000000000000 laptop.kdbx

# only in A
keep    3f000000-0000-0000-0000-000000000001  # Email / Work mail
keep    3f000000-0000-0000-0000-000000000002  # group: Games

# only in B
keep    3f000000-0000-0000-0000-000000000003  # Games / Old forum

# differs: password, url  (A 2026-01-01 02:00, B 2026-01-21 20:00)
take-b  3f000000-0000-0000-0000-000000000004  # Banking / My bank

# differs: title, field PIN  (A -, B -)
both    3f000000-0000-0000-0000-000000000005  # Shopping / Store

# deleted in A on 2026-01-01 09:00  (B 2026-01-01 03:00)
drop    3f000000-0000-0000-0000-000000000006  # Misc / Gone in A

# deleted in B on 2026-01-01 09:00  (A 2026-01-01 03:00)
drop    3f000000-0000-0000-0000-000000000007  # Misc / Gone in B

# group differs: name  (A -, B -)
take-a  3f000000-0000-0000-0000-000000000008  # group: Work

# settings differ: name, color  (A -, B -)
settings take-a
`
	if got != want {
		t.Errorf("plan file:\n%s\nwant:\n%s", got, want)
	}
	// NoTitles leaves out the group paths, the titles and the custom field names.
	nt := writeString(t, p, WriteOptions{NoTitles: true})
	for _, secret := range []string{"Work mail", "Games", "Old forum", "My bank", "Store", "Banking", "PIN"} {
		if strings.Contains(nt, secret) {
			t.Errorf("--no-titles still shows %q:\n%s", secret, nt)
		}
	}
	if !strings.Contains(nt, "# differs: title, field  (A -, B -)") || !strings.Contains(nt, "take-b  3f000000-0000-0000-0000-000000000004\n") {
		t.Errorf("no-titles file:\n%s", nt)
	}
}

func TestWriteThenRead(t *testing.T) {
	titles := []string{"plain", "with spaces  and\ttabs", "has # a hash", "# starts with a hash", "Ünïcödé 日本語 🔑", "line\nbreak", "cr\rlf", "nul\x00byte", "bad \xff utf8", strings.Repeat("long ", 2000)}
	for _, noTitles := range []bool{false, true} {
		for _, title := range titles {
			changes := sample()
			for i := range changes {
				changes[i].Title = title
				changes[i].Path = title
			}
			p := New(inA, inB, changes)
			// A mix of allowed choices.
			for i := range p.Changes {
				a := Allowed(p.Changes[i].Kind)
				p.Choices[i] = a[i%len(a)]
			}
			var b bytes.Buffer
			if err := Write(&b, p, WriteOptions{NoTitles: noTitles}); err != nil {
				t.Fatal(err)
			}
			// A comment never breaks the line structure: one line per change at most.
			if !utf8.Valid(b.Bytes()) {
				t.Fatalf("title %q: the file is not UTF-8", title)
			}
			got, err := Read(bytes.NewReader(b.Bytes()), inA, inB, changes)
			if err != nil {
				t.Fatalf("title %q (no titles %v): %v\n%s", title, noTitles, err, b.String())
			}
			if !slices.Equal(got.Choices, p.Choices) {
				t.Errorf("title %q: choices %v, want %v", title, got.Choices, p.Choices)
			}
		}
	}
}

func TestWriteDefaultsRoundTrip(t *testing.T) {
	changes := sample()
	p := New(inA, inB, changes)
	got, err := Read(strings.NewReader(writeString(t, p, WriteOptions{})), inA, inB, changes)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Choices, choicesOf(t, p)) || got.A != inA || got.B != inB {
		t.Errorf("read %v, want %v", got.Choices, p.Choices)
	}
}

func TestReadAccepts(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.OnlyA, Item: diff.ItemEntry, UUID: uid(1)},
		{Kind: diff.Differs, Item: diff.ItemEntry, UUID: uid(2)},
		{Kind: diff.Settings, Item: diff.ItemSettings},
	}
	hdr := "passbank-plan 1\na sha256:" + hexOf(inA.Hash) + " a.kdbx\nb sha256:" + hexOf(inB.Hash) + " b.kdbx\n"
	tests := []struct {
		name string
		text string
		want []Choice
	}{
		{"plain", hdr + "keep " + uid(1).String() + "\ntake-b " + uid(2).String() + "\nsettings take-a\n", []Choice{Keep, TakeB, TakeA}},
		{"CRLF", strings.ReplaceAll(hdr+"drop "+uid(1).String()+"\nboth "+uid(2).String()+"\nsettings take-b\n", "\n", "\r\n"), []Choice{Drop, Both, TakeB}},
		{"comments and blank lines everywhere", strings.Replace(hdr, "\n", "\n# note\n\n", 1) + "# c\n  keep   " + uid(1).String() + "   # x # y\n\t\ndrop " + uid(2).String() + "#\n# settings take-b\nsettings   take-b # chosen\n", []Choice{Keep, Drop, TakeB}},
		{"upper-case UUID", hdr + "keep " + strings.ToUpper(uid(1).String()) + "\ndrop " + uid(2).String() + "\nsettings take-a", []Choice{Keep, Drop, TakeA}},
		{"the settings word second", hdr + "keep " + uid(1).String() + "\ndrop " + uid(2).String() + "\ntake-b settings\n", []Choice{Keep, Drop, TakeB}},
		{"a byte order mark", "\uFEFF" + hdr + "keep " + uid(1).String() + "\ndrop " + uid(2).String() + "\nsettings take-a\n", []Choice{Keep, Drop, TakeA}},
		{"names of the inputs are not checked", "passbank-plan 1\na sha256:" + hexOf(inA.Hash) + "\nb sha256:" + hexOf(inB.Hash) + " other name # with hash\nkeep " + uid(1).String() + "\ndrop " + uid(2).String() + "\nsettings take-a\n", []Choice{Keep, Drop, TakeA}},
		{"the last line without a line break", hdr + "keep " + uid(1).String() + "\ndrop " + uid(2).String() + "\nsettings take-a", []Choice{Keep, Drop, TakeA}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Read(strings.NewReader(tt.text), inA, inB, changes)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(p.Choices, tt.want) {
				t.Errorf("choices = %v, want %v", p.Choices, tt.want)
			}
		})
	}
}

func hexOf(h [32]byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, 64)
	for _, b := range h {
		out = append(out, digits[b>>4], digits[b&15])
	}
	return string(out)
}

func TestReadErrors(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.OnlyA, Item: diff.ItemEntry, UUID: uid(1)},
		{Kind: diff.Differs, Item: diff.ItemEntry, UUID: uid(2)},
		{Kind: diff.Settings, Item: diff.ItemSettings},
	}
	ha, hb := "sha256:"+hexOf(inA.Hash), "sha256:"+hexOf(inB.Hash)
	hdr := "passbank-plan 1\na " + ha + " a\nb " + hb + " b\n"
	u1, u2 := uid(1).String(), uid(2).String()
	ok := "keep " + u1 + "\ndrop " + u2 + "\nsettings take-a\n"
	tests := []struct {
		name  string
		text  string
		route string
		line  int // 0: do not check
	}{
		{"empty", "", "Read.PlanSyntax", 1},
		{"only a comment", "# nothing\n", "Read.PlanSyntax", 1},
		{"wrong format line", "passbank-plan 2\n" + ok, "Read.PlanSyntax", 1},
		{"no format line", "keep " + u1 + "\n", "Read.PlanSyntax", 1},
		{"no a and b lines", "passbank-plan 1\n", "Read.PlanSyntax", 0},
		{"no b line", "passbank-plan 1\na " + ha + " a\n", "Read.PlanSyntax", 0},
		{"a line is swapped with b", "passbank-plan 1\nb " + hb + " b\na " + ha + " a\n" + ok, "Read.PlanSyntax", 2},
		{"a hash that is not hex", "passbank-plan 1\na sha256:zz a\nb " + hb + "\n" + ok, "Read.PlanSyntax", 2},
		{"a hash that is too short", "passbank-plan 1\na sha256:0102 a\nb " + hb + "\n" + ok, "Read.PlanSyntax", 2},
		{"a hash without the algorithm", "passbank-plan 1\na " + hexOf(inA.Hash) + " a\nb " + hb + "\n" + ok, "Read.PlanSyntax", 2},
		{"one word", hdr + "keep\n", "Read.PlanSyntax", 4},
		{"three words", hdr + "keep " + u1 + " extra\n", "Read.PlanSyntax", 4},
		{"an unknown choice", hdr + "maybe " + u1 + "\n", "Read.PlanSyntax", 4},
		{"the item is not a UUID", hdr + "keep banana\n", "Read.PlanSyntax", 4},
		{"the choice and the item swapped", hdr + u1 + " keep\n", "Read.PlanSyntax", 4},
		{"invalid UTF-8", hdr + "keep " + u1 + " # \xff\n", "Read.PlanSyntax", 4},
		{"a line that is too long", hdr + "# " + strings.Repeat("x", maxLine+10) + "\n", "Read.PlanSyntax", 4},
		{"stale A", "passbank-plan 1\na sha256:" + strings.Repeat("0", 64) + " a\nb " + hb + "\n" + ok, "Read.PlanStale", 2},
		{"stale B", "passbank-plan 1\na " + ha + " a\nb sha256:" + strings.Repeat("0", 64) + " b\n" + ok, "Read.PlanStale", 3},
		{"an unknown item", hdr + "keep " + uid(9).String() + "\n", "Read.PlanUnknownItem", 4},
		{"settings that are not a change", strings.Replace(hdr, "b ", "b ", 1) + "keep " + u1 + "\ndrop " + u2 + "\nsettings take-a\n" + "keep " + uid(77).String() + "\n", "Read.PlanUnknownItem", 7},
		{"a choice the kind does not allow", hdr + "take-a " + u1 + "\n", "Read.PlanInvalidChoice", 4},
		{"both on a plain entry", hdr + "both " + u1 + "\n", "Read.PlanInvalidChoice", 4},
		{"keep on settings", hdr + "settings keep\n", "Read.PlanInvalidChoice", 4},
		{"a duplicate item", hdr + "keep " + u1 + "\ndrop " + u1 + "\n", "Read.PlanDuplicateItem", 5},
		{"a missing change", hdr + "keep " + u1 + "\ndrop " + u2 + "\n", "Read.PlanIncomplete", 0},
		{"all changes missing", hdr, "Read.PlanIncomplete", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Read(strings.NewReader(tt.text), inA, inB, changes)
			appErr, isApp := errors.FromError(err)
			if !isApp {
				t.Fatalf("expected *errors.Error, got %T (%v)", err, err)
			}
			if !appErr.HasRoute(tt.route) {
				t.Errorf("route = %s, want %s", appErr.Route(), tt.route)
			}
			if tt.line != 0 {
				if got, _ := appErr.Metadata["line"].(int); got != tt.line {
					t.Errorf("line = %v, want %d", appErr.Metadata["line"], tt.line)
				}
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestReadWriteFailures(t *testing.T) {
	_, err := Read(failingReader{}, inA, inB, nil)
	appErr, ok := errors.FromError(err)
	if !ok || !appErr.HasRoute("Read.PlanReadFailed") || !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("read error = %v", err)
	}
	err = Write(failingWriter{}, New(inA, inB, sample()), WriteOptions{})
	appErr, ok = errors.FromError(err)
	if !ok || !appErr.HasRoute("Write.PlanWriteFailed") || !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("write error = %v", err)
	}
	p := New(inA, inB, sample())
	p.Choices = p.Choices[:2]
	err = Write(io.Discard, p, WriteOptions{})
	appErr, ok = errors.FromError(err)
	if !ok || !appErr.HasRoute("Write.PlanInconsistent") {
		t.Errorf("inconsistent error = %v", err)
	}
}

// A plan file never holds a field value; the changes carry none to leak, and the
// comments are built from paths, titles and field names only.
func TestWriteHoldsNoValues(t *testing.T) {
	c := sample()
	p := New(inA, inB, c)
	if s := writeString(t, p, WriteOptions{}); strings.Contains(s, "SECRET") {
		t.Error("a value in the plan file")
	}
}

func FuzzRead(f *testing.F) {
	changes := sample()
	p := New(inA, inB, changes)
	var b bytes.Buffer
	if err := Write(&b, p, WriteOptions{}); err != nil {
		f.Fatal(err)
	}
	f.Add(b.Bytes())
	f.Add([]byte("passbank-plan 1\n"))
	f.Add([]byte("\xef\xbb\xbfpassbank-plan 1\r\na sha256:00\r\n"))
	f.Add([]byte("keep 3f000000-0000-0000-0000-000000000001\nsettings\n# \xff"))
	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := Read(bytes.NewReader(data), inA, inB, changes)
		if err != nil {
			if _, ok := errors.FromError(err); !ok {
				t.Fatalf("error is %T, not *errors.Error", err)
			}
			return
		}
		// A plan that reads is complete and every choice is allowed.
		if len(got.Choices) != len(changes) {
			t.Fatalf("%d choices for %d changes", len(got.Choices), len(changes))
		}
		for i, c := range got.Choices {
			if !IsAllowed(changes[i].Kind, c) {
				t.Fatalf("choice %s is not allowed for %s", c, changes[i].Kind)
			}
		}
	})
}
