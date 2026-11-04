package plan

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model"
)

// FormatLine is the first line of a plan file.
const FormatLine = "passbank-plan 1"

// maxLine is the longest line Read accepts.
const maxLine = 1 << 20

// byteOrderMark may start a file that an editor saved.
const byteOrderMark = "\uFEFF"

// settingsItem is the item word of the settings change.
const settingsItem = "settings"

// WriteOptions are the options of Write.
type WriteOptions struct {
	// NoTitles leaves out the group paths, titles and custom field names that the
	// comments would show, because titles can be sensitive.
	NoTitles bool
}

// Write writes the plan as the plan file of SPEC §11: the header, then one line
// per change in the order of the plan, each with a comment that shows what it is.
// The file never holds a password or any other field value.
func Write(w io.Writer, p *Plan, opts WriteOptions) error {
	if len(p.Choices) != len(p.Changes) {
		return errors.New("PlanInconsistent", "the plan has a different number of choices and changes",
			"choices", len(p.Choices), "changes", len(p.Changes))
	}
	var b bytes.Buffer
	b.WriteString(FormatLine + "\n")
	b.WriteString(inputLine("a", p.A) + "\n")
	b.WriteString(inputLine("b", p.B) + "\n")
	prev := diff.Kind(-1)
	for i, c := range p.Changes {
		if c.Kind == diff.OnlyA || c.Kind == diff.OnlyB {
			if c.Kind != prev {
				b.WriteString("\n# " + sectionName(c.Kind) + "\n")
			}
		} else {
			b.WriteString("\n# " + changeComment(c, opts) + "\n")
		}
		prev = c.Kind
		choice := string(p.Choices[i])
		if c.Item == diff.ItemSettings {
			b.WriteString(settingsItem + " " + choice + "\n")
			continue
		}
		line := pad(choice) + " " + c.UUID.String()
		if title := titleComment(c, opts); title != "" {
			line += "  # " + title
		}
		b.WriteString(line + "\n")
	}
	if _, err := w.Write(b.Bytes()); err != nil {
		return errors.NewErr("PlanWriteFailed", err)
	}
	return nil
}

func pad(s string) string {
	for len(s) < 7 {
		s += " "
	}
	return s
}

func inputLine(key string, in Input) string {
	return key + " sha256:" + hex.EncodeToString(in.Hash[:]) + " " + clean(in.Name)
}

func sectionName(k diff.Kind) string {
	if k == diff.OnlyA {
		return "only in A"
	}
	return "only in B"
}

// changeComment is the comment above a change that is not "only in".
func changeComment(c diff.Change, opts WriteOptions) string {
	times := "(A " + stamp(c.Modified[diff.SideA]) + ", B " + stamp(c.Modified[diff.SideB]) + ")"
	fields := fieldComment(c.Fields, opts)
	switch c.Kind {
	case diff.Differs:
		return "differs: " + fields + "  " + times
	case diff.GroupDiffers:
		return "group differs: " + fields + "  " + times
	case diff.DeletedInA:
		return "deleted in A on " + stamp(c.Deleted) + "  (B " + stamp(c.Modified[diff.SideB]) + ")"
	case diff.DeletedInB:
		return "deleted in B on " + stamp(c.Deleted) + "  (A " + stamp(c.Modified[diff.SideA]) + ")"
	case diff.Settings:
		return "settings differ: " + fields + "  " + times
	}
	return c.Kind.String()
}

// fieldComment joins the names of the differing fields; the names of custom
// fields are left out with NoTitles.
func fieldComment(fields []string, opts WriteOptions) string {
	out := make([]string, len(fields))
	for i, f := range fields {
		if opts.NoTitles && strings.HasPrefix(f, "field ") {
			f = "field"
		}
		out[i] = clean(f)
	}
	return strings.Join(out, ", ")
}

// titleComment is the group path and title of a change, or "" with NoTitles.
func titleComment(c diff.Change, opts WriteOptions) string {
	if opts.NoTitles {
		return ""
	}
	s := c.Title
	if c.Path != "" {
		s = c.Path + " / " + c.Title
	}
	if c.Item == diff.ItemGroup {
		s = "group: " + s
	}
	return clean(s)
}

// stamp formats a time for a comment: UTC, to the minute; empty for no time.
func stamp(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04")
}

// clean makes text safe to put on a line of a plan file: no line breaks or other
// control characters, and valid UTF-8.
func clean(s string) string {
	s = strings.ToValidUTF8(s, "�")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return ' '
		}
		return r
	}, s)
}

// key is how a plan file names a change: its UUID, or "settings".
func key(c diff.Change) string {
	if c.Item == diff.ItemSettings {
		return settingsItem
	}
	return c.UUID.String()
}

// Read reads a plan file for the inputs a and b (their hashes are compared with
// the file's) and the changes between them. Every change must be named exactly
// once with a choice its kind allows. Comments (from # to the end of the line)
// and blank lines are ignored, and CRLF line endings are accepted.
//
// The errors are PlanSyntax (with its line number), PlanStale, PlanUnknownItem,
// PlanInvalidChoice, PlanDuplicateItem and PlanIncomplete (SPEC §11).
func Read(r io.Reader, a, b Input, changes []diff.Change) (*Plan, error) {
	index := make(map[string]int, len(changes))
	for i, c := range changes {
		index[key(c)] = i
	}
	choices := make([]Choice, len(changes))
	seen := 0

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxLine)
	line := 0
	stage := 0 // 0: format line, 1: line a, 2: line b, 3: changes
	for sc.Scan() {
		line++
		text := sc.Text()
		if line == 1 {
			text = strings.TrimPrefix(text, byteOrderMark)
		}
		if !utf8.ValidString(text) {
			return nil, errors.New("PlanSyntax", "the line is not valid UTF-8", "line", line)
		}
		text = strings.TrimRight(text, "\r")
		switch stage {
		case 0:
			if strings.TrimSpace(text) != FormatLine {
				return nil, errors.New("PlanSyntax", "the first line must be "+strconv.Quote(FormatLine), "line", line)
			}
			stage = 1
			continue
		case 1, 2:
			if blank(text) {
				continue
			}
			which, want := "a", a
			if stage == 2 {
				which, want = "b", b
			}
			hash, ok := parseInputLine(text, which)
			if !ok {
				return nil, errors.New("PlanSyntax", "expected the line `"+which+" sha256:<hash> <name>`", "line", line)
			}
			if hash != want.Hash {
				return nil, errors.New("PlanStale", "the plan was made for another version of input "+strings.ToUpper(which),
					"input", strings.ToUpper(which), "line", line)
			}
			stage++
			continue
		}
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = text[:i]
		}
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, errors.New("PlanSyntax", "expected `<choice> <item>`", "line", line)
		}
		itemWord, choiceWord := fields[1], fields[0]
		if fields[0] == settingsItem {
			itemWord, choiceWord = fields[0], fields[1]
		}
		choice, ok := ParseChoice(choiceWord)
		if !ok {
			return nil, errors.New("PlanSyntax", "the first word must be a choice: keep, drop, take-a, take-b or both", "line", line)
		}
		if itemWord != settingsItem {
			u, err := model.ParseUUID(itemWord)
			if err != nil {
				return nil, errors.New("PlanSyntax", "the second word must be a UUID or settings", "line", line)
			}
			itemWord = u.String()
		}
		i, ok := index[itemWord]
		if !ok {
			return nil, errors.New("PlanUnknownItem", "the item is not a change between these inputs", "item", itemWord, "line", line)
		}
		if !IsAllowed(changes[i].Kind, choice) {
			return nil, errors.New("PlanInvalidChoice", "the choice is not allowed for this kind of change",
				"choice", string(choice), "kind", changes[i].Kind.String(), "item", itemWord, "line", line)
		}
		if choices[i] != "" {
			return nil, errors.New("PlanDuplicateItem", "the item has more than one line", "item", itemWord, "line", line)
		}
		choices[i] = choice
		seen++
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, errors.New("PlanSyntax", "the line is too long", "line", line+1)
		}
		return nil, errors.NewErr("PlanReadFailed", err)
	}
	if stage == 0 {
		return nil, errors.New("PlanSyntax", "the plan is empty; the first line must be "+strconv.Quote(FormatLine), "line", 1)
	}
	if stage < 3 {
		return nil, errors.New("PlanSyntax", "the lines `a` and `b` are missing", "line", line+1)
	}
	if seen < len(changes) {
		missing := ""
		for i, c := range changes {
			if choices[i] == "" {
				missing = key(c)
				break
			}
		}
		return nil, errors.New("PlanIncomplete", "the plan has no line for some changes",
			"missing", len(changes)-seen, "first", missing)
	}
	return &Plan{A: a, B: b, Changes: changes, Choices: choices}, nil
}

func blank(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || strings.HasPrefix(s, "#")
}

// parseInputLine reads `<key> sha256:<64 hex digits> [name]` and returns the hash, or false when the line is not like that.
func parseInputLine(text, key string) ([32]byte, bool) {
	var out [32]byte
	fields := strings.Fields(text)
	if len(fields) < 2 || fields[0] != key || !strings.HasPrefix(fields[1], "sha256:") {
		return out, false
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(fields[1], "sha256:"))
	if err != nil || len(raw) != len(out) {
		return out, false
	}
	copy(out[:], raw)
	return out, true
}
