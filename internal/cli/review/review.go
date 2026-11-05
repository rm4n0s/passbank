// Package review is the interactive part of `passbank merge`: it goes through the
// changes between two databases one by one, shows each, and asks the user what to
// keep (SPEC §12.1). It reads lines from a terminal and writes to it; it knows
// nothing of files or databases beyond the plan it changes.
package review

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/merge"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

// Terminal is the terminal the user answers on.
type Terminal interface {
	// ReadLine shows the prompt and reads a line without its line break; io.EOF at
	// the end of the input.
	ReadLine(prompt string) (string, error)
	// Write shows text.
	io.Writer
}

// Outcome says how the review ended.
type Outcome int

const (
	// Write means the user went through the changes and agreed to write the result.
	Write Outcome = iota
	// Quit means the user stopped: nothing is to be written.
	Quit
)

// Options are the options of Run.
type Options struct {
	// Out is the path that the result will be written to, for the last question.
	Out string
}

// Run goes through the changes of the plan, shows each with its default, and sets
// the plan's choices to the user's answers. It ends with the summary and the
// question whether to write the result. The plan is changed in place and stays
// valid (merge.Validate) when Run returns Write. A read error other than the end
// of the input is returned; the end of the input is Quit.
func Run(t Terminal, p *plan.Plan, opts Options) (Outcome, error) {
	s := &session{t: t, p: p, out: opts.Out}
	out, err := s.run()
	if s.werr != nil {
		return Quit, errors.NewErr("TerminalWriteFailed", s.werr)
	}
	return out, err
}

// session is one review.
type session struct {
	t    Terminal
	p    *plan.Plan
	out  string
	werr error // the first error writing to the terminal
}

func (s *session) printf(format string, args ...any) {
	if s.werr == nil {
		_, s.werr = io.WriteString(s.t, fmt.Sprintf(format, args...))
	}
}

// ask reads a line; ok is false at the end of the input.
func (s *session) ask(prompt string) (line string, ok bool, err error) {
	line, err = s.t.ReadLine(prompt)
	if errors.Is(err, io.EOF) {
		s.printf("\n")
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(line), true, nil
}

func (s *session) run() (Outcome, error) {
	n := len(s.p.Changes)
	for i := 0; i < n; {
		s.show(i)
		line, ok, err := s.ask("choice> ")
		if err != nil {
			return Quit, err
		}
		if !ok {
			return Quit, nil
		}
		switch key := strings.ToLower(line); key {
		case "":
			// Enter takes the choice shown: the default of SPEC §8, unless an earlier
			// answer (or dropping a group) changed it.
			i++
		case "?":
			s.help()
		case "q":
			return Quit, nil
		case "p":
			if i == 0 {
				s.printf("  this is the first change\n")
			} else {
				i--
			}
		case "v":
			s.values(i)
		case "r":
			applied, err := s.rule(i)
			if err != nil {
				return Quit, err
			}
			if applied {
				i = n
			}
		default:
			choice, ok := keyChoice(s.p.Changes[i].Kind, key)
			if !ok {
				s.printf("  %q is not one of the keys; ? shows them\n", line)
				continue
			}
			applied, err := s.choose(i, choice)
			if err != nil {
				return Quit, err
			}
			if applied {
				i++
			}
		}
	}
	if err := s.resolveRemaining(); err != nil {
		return Quit, err
	}
	return s.finish()
}

// keyChoice is the choice that a key stands for in a kind of change.
func keyChoice(kind diff.Kind, key string) (plan.Choice, bool) {
	var c plan.Choice
	switch key {
	case "a":
		c = plan.TakeA
	case "b":
		c = plan.TakeB
	case "t":
		c = plan.Both
	case "k":
		c = plan.Keep
	case "d":
		c = plan.Drop
	default:
		return "", false
	}
	return c, plan.IsAllowed(kind, c)
}

// keyFor is the key of a choice.
func keyFor(c plan.Choice) string {
	switch c {
	case plan.TakeA:
		return "a"
	case plan.TakeB:
		return "b"
	case plan.Both:
		return "t"
	case plan.Keep:
		return "k"
	}
	return "d"
}

var choiceWords = map[plan.Choice]string{
	plan.TakeA: "take A", plan.TakeB: "take B", plan.Both: "keep both", plan.Keep: "keep", plan.Drop: "drop",
}

// show prints a change with its keys.
func (s *session) show(i int) {
	c := s.p.Changes[i]
	s.printf("\n[%d/%d] %s  %s\n", i+1, len(s.p.Changes), c.Kind, displayName(c))
	switch c.Kind {
	case diff.OnlyA:
		s.printf("  only in A, modified %s\n", stamp(c.Modified[diff.SideA]))
	case diff.OnlyB:
		s.printf("  only in B, modified %s\n", stamp(c.Modified[diff.SideB]))
	case diff.DeletedInA:
		s.printf("  A deleted it on %s; B modified it %s\n", stamp(c.Deleted), stamp(c.Modified[diff.SideB]))
	case diff.DeletedInB:
		s.printf("  B deleted it on %s; A modified it %s\n", stamp(c.Deleted), stamp(c.Modified[diff.SideA]))
	case diff.Differs, diff.GroupDiffers:
		s.printf("  A modified %s, B modified %s\n", stamp(c.Modified[diff.SideA]), stamp(c.Modified[diff.SideB]))
	case diff.Settings:
		if !c.Modified[diff.SideA].IsZero() || !c.Modified[diff.SideB].IsZero() {
			s.printf("  A changed the settings %s, B %s\n", stamp(c.Modified[diff.SideA]), stamp(c.Modified[diff.SideB]))
		}
	}
	if len(c.Fields) > 0 {
		s.printf("  differs: %s\n", c.FieldList())
	}
	s.showFields(c)

	var keys []string
	for _, ch := range plan.Allowed(c.Kind) {
		keys = append(keys, "["+keyFor(ch)+"] "+choiceWords[ch])
	}
	enter := choiceWords[s.p.Choices[i]]
	if def := plan.DefaultFor(c); s.p.Choices[i] != def {
		enter += " (default: " + choiceWords[def] + ")"
	}
	s.printf("  %s   Enter = %s\n", strings.Join(keys, "  "), enter)
}

// displayName is the group path and title of a change.
func displayName(c diff.Change) string {
	switch c.Item {
	case diff.ItemSettings:
		return "database settings"
	case diff.ItemGroup:
		return "group " + joinPath(c.Path, c.Title)
	}
	return joinPath(c.Path, c.Title)
}

func joinPath(path, title string) string {
	if path == "" {
		return title
	}
	return path + " / " + title
}

// stamp formats a time to the minute in UTC.
func stamp(t time.Time) string {
	if t.IsZero() {
		return "(no time)"
	}
	return t.UTC().Format("2006-01-02 15:04")
}

// shown are the standard fields whose values are printed: they are not secret.
var shown = []struct{ label, key string }{
	{"title", model.FieldTitle}, {"username", model.FieldUserName}, {"url", model.FieldURL},
}

// showFields prints the values of the non-secret standard fields that differ, side
// by side; for an item on one side only, the user name and URL it has. Everything
// else is shown by name only (SPEC §12.1).
func (s *session) showFields(c diff.Change) {
	if c.Item == diff.ItemGroup && c.Kind == diff.GroupDiffers {
		a, b := c.Group[diff.SideA], c.Group[diff.SideB]
		if a.Name != b.Name {
			s.printf("  name   A: %s   B: %s\n", clean(a.Name), clean(b.Name))
		}
		return
	}
	if c.Item != diff.ItemEntry {
		return
	}
	ea, eb := c.Entry[diff.SideA], c.Entry[diff.SideB]
	for _, f := range shown {
		switch {
		case ea != nil && eb != nil:
			if !contains(c.Fields, f.label) {
				continue
			}
			s.printf("  %-8s A: %s   B: %s\n", f.label, clean(text(ea, f.key)), clean(text(eb, f.key)))
		default:
			e := ea
			if e == nil {
				e = eb
			}
			if f.key != model.FieldTitle && text(e, f.key) != "" {
				s.printf("  %-8s %s\n", f.label, clean(text(e, f.key)))
			}
		}
	}
}

func text(e *model.Entry, key string) string {
	f, _ := e.Field(key)
	return f.Text()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// clean makes a value safe to print on one line: control characters, such as an
// escape sequence, become spaces.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return ' '
		}
		return r
	}, s)
}

// values prints the values that differ and that show() leaves out: the password,
// the notes and the custom fields. They are printed on the terminal once and kept
// nowhere.
func (s *session) values(i int) {
	c := s.p.Changes[i]
	ea, eb := c.Entry[diff.SideA], c.Entry[diff.SideB]
	if c.Item != diff.ItemEntry || ea == nil || eb == nil {
		s.printf("  no values differ here\n")
		return
	}
	printed := false
	for _, label := range c.Fields {
		var key string
		switch {
		case label == "password":
			key = model.FieldPassword
		case label == "notes":
			key = model.FieldNotes
		case strings.HasPrefix(label, "field "):
			key = strings.TrimPrefix(label, "field ")
		default:
			continue
		}
		s.printf("  %s\n    A: %s\n    B: %s\n", label, clean(text(ea, key)), clean(text(eb, key)))
		printed = true
	}
	if !printed {
		s.printf("  no password, notes or custom field differs\n")
	}
}

func (s *session) help() {
	s.printf("  a  take A's version          b  take B's version\n")
	s.printf("  t  keep both (B's copy gets a new UUID)\n")
	s.printf("  k  keep it                   d  drop it\n")
	s.printf("  Enter  the choice shown (the default)   v  show the password, notes and custom fields that differ\n")
	s.printf("  p  go back                   r  apply a rule to this and all the remaining changes\n")
	s.printf("  q  quit; nothing is written\n")
}

// rule asks for a rule and applies it to the change and all the remaining ones.
func (s *session) rule(from int) (applied bool, err error) {
	for {
		line, ok, err := s.ask("rule (newer, a, b, union; Enter cancels)> ")
		if err != nil {
			return false, err
		}
		if !ok || line == "" {
			return false, nil
		}
		var rule plan.Rule
		switch strings.ToLower(line) {
		case "newer", "n":
			rule = plan.Newer
		case "a":
			rule = plan.PreferA
		case "b":
			rule = plan.PreferB
		case "union", "u":
			rule = plan.Union
		default:
			s.printf("  %q is not a rule\n", line)
			continue
		}
		plan.ApplyRule(s.p, rule, func(i int, _ diff.Change) bool { return i >= from })
		s.printf("  applied the rule %s to %d changes\n", rule, len(s.p.Changes)-from)
		return true, nil
	}
}

// choose sets the choice of change i. Dropping a group that still holds kept items
// asks whether to drop them too; keeping an item inside a group that is dropped
// asks whether to keep the group. When the answer is no, the choice is not made
// (applied is false) and the user is asked again.
func (s *session) choose(i int, choice plan.Choice) (applied bool, err error) {
	prev := s.p.Choices[i]
	s.p.Choices[i] = choice
	for {
		conflicts := merge.Conflicts(s.p)
		if len(conflicts) == 0 {
			return true, nil
		}
		c := conflicts[0]
		switch {
		case c.Group == i:
			n := 0
			for _, x := range conflicts {
				if x.Group == i {
					n++
				}
			}
			yes, ok, err := s.confirm("  Also drop the " + strconv.Itoa(n) + " items inside " + displayName(s.p.Changes[i]) + "? [y/N] ")
			if err != nil {
				return false, err
			}
			if !yes || !ok {
				s.p.Choices[i] = prev
				s.printf("  the group stays\n")
				return false, nil
			}
			for _, x := range conflicts {
				s.dropOrMove(x.Item)
			}
		case c.Item == i:
			g := s.p.Changes[c.Group]
			yes, ok, err := s.confirmDefaultYes("  " + displayName(g) + " is dropped, and this choice keeps something in it. Keep the group too? [Y/n] ")
			if err != nil {
				return false, err
			}
			if !yes || !ok {
				s.p.Choices[i] = prev
				s.printf("  unchanged\n")
				return false, nil
			}
			s.p.Choices[c.Group] = plan.Keep
		default:
			// A consequence of dropping the group: what is inside the items dropped.
			s.dropOrMove(c.Item)
		}
	}
}

// dropOrMove takes an item out of a dropped group: it is dropped, and a group that
// both inputs have takes the place the other input gave it.
func (s *session) dropOrMove(item int) {
	c := s.p.Changes[item]
	if plan.IsAllowed(c.Kind, plan.Drop) {
		s.p.Choices[item] = plan.Drop
		return
	}
	if s.p.Choices[item] == plan.TakeA {
		s.p.Choices[item] = plan.TakeB
	} else {
		s.p.Choices[item] = plan.TakeA
	}
}

// resolveRemaining settles the conflicts that are left after a rule: for each, it
// asks whether to keep the group or to drop what is in it.
func (s *session) resolveRemaining() error {
	for {
		conflicts := merge.Conflicts(s.p)
		if len(conflicts) == 0 {
			return nil
		}
		c := conflicts[0]
		g, it := s.p.Changes[c.Group], s.p.Changes[c.Item]
		yes, ok, err := s.confirmDefaultYes("\n" + displayName(g) + " is dropped, but " + displayName(it) + " in it is kept. Keep the group? [Y/n] ")
		if err != nil {
			return err
		}
		if yes || !ok {
			s.p.Choices[c.Group] = plan.Keep
		} else {
			s.dropOrMove(c.Item)
		}
	}
}

// finish prints the summary and asks whether to write the result.
func (s *session) finish() (Outcome, error) {
	s.printf("\n")
	for _, line := range s.p.Summary() {
		s.printf("%s\n", line)
	}
	yes, _, err := s.confirm("Write " + s.out + "? [y/N] ")
	if err != nil {
		return Quit, err
	}
	if yes {
		return Write, nil
	}
	return Quit, nil
}

// confirm asks a question whose default answer is no.
func (s *session) confirm(prompt string) (yes, ok bool, err error) {
	line, ok, err := s.ask(prompt)
	if err != nil || !ok {
		return false, ok, err
	}
	l := strings.ToLower(line)
	return l == "y" || l == "yes", true, nil
}

// confirmDefaultYes asks a question whose default answer is yes.
func (s *session) confirmDefaultYes(prompt string) (yes, ok bool, err error) {
	line, ok, err := s.ask(prompt)
	if err != nil || !ok {
		return false, ok, err
	}
	l := strings.ToLower(line)
	return l == "" || l == "y" || l == "yes", true, nil
}
