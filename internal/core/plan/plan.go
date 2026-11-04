package plan

import (
	"github.com/rm4n0s/passbank/internal/core/diff"
)

// Choice is what the user decides for one change (SPEC §8).
type Choice string

// The choices.
const (
	// Keep keeps an item that exists in one input only.
	Keep Choice = "keep"
	// Drop leaves the item out of the result.
	Drop Choice = "drop"
	// TakeA takes A's version of the item.
	TakeA Choice = "take-a"
	// TakeB takes B's version of the item.
	TakeB Choice = "take-b"
	// Both keeps both entries: A's copy as it is, B's copy with a new UUID.
	Both Choice = "both"
)

// Rule is a way to set many choices at once (SPEC §9).
type Rule string

// The rules.
const (
	// Newer sets the defaults of SPEC §8.
	Newer Rule = "newer"
	// PreferA lets A win everywhere.
	PreferA Rule = "a"
	// PreferB lets B win everywhere.
	PreferB Rule = "b"
	// Union keeps everything that exists in one input only and both versions of
	// every pair that differs.
	Union Rule = "union"
)

// Rules lists the rules.
var Rules = []Rule{Newer, PreferA, PreferB, Union}

// ParseRule returns the rule with the given name.
func ParseRule(s string) (Rule, bool) {
	for _, r := range Rules {
		if string(r) == s {
			return r, true
		}
	}
	return "", false
}

// ParseChoice returns the choice with the given name.
func ParseChoice(s string) (Choice, bool) {
	for _, c := range []Choice{Keep, Drop, TakeA, TakeB, Both} {
		if string(c) == s {
			return c, true
		}
	}
	return "", false
}

// Input identifies one input file: the SHA-256 of its bytes and its name.
type Input struct {
	Hash [32]byte
	Name string
}

// Plan holds the changes between two inputs and one choice for each.
type Plan struct {
	A, B    Input
	Changes []diff.Change
	// Choices[i] is the choice for Changes[i].
	Choices []Choice
}

// New makes a plan for the changes with the default choice for each.
func New(a, b Input, changes []diff.Change) *Plan {
	return &Plan{A: a, B: b, Changes: changes, Choices: Default(changes)}
}

// Allowed returns the choices a kind of change allows, as the table of SPEC §8
// lists them.
func Allowed(kind diff.Kind) []Choice {
	switch kind {
	case diff.OnlyA, diff.OnlyB, diff.DeletedInA, diff.DeletedInB:
		return []Choice{Keep, Drop}
	case diff.Differs:
		return []Choice{TakeA, TakeB, Both, Drop}
	case diff.GroupDiffers, diff.Settings:
		return []Choice{TakeA, TakeB}
	}
	return nil
}

// IsAllowed reports whether the kind of change allows the choice.
func IsAllowed(kind diff.Kind, c Choice) bool {
	for _, a := range Allowed(kind) {
		if a == c {
			return true
		}
	}
	return false
}

// Set changes the choice of change i. It reports false, and changes nothing, when
// i is out of range or the kind of that change does not allow the choice.
func (p *Plan) Set(i int, c Choice) bool {
	if i < 0 || i >= len(p.Changes) || !IsAllowed(p.Changes[i].Kind, c) {
		return false
	}
	p.Choices[i] = c
	return true
}

// Default returns the default choice of every change.
func Default(changes []diff.Change) []Choice {
	out := make([]Choice, len(changes))
	for i, c := range changes {
		out[i] = DefaultFor(c)
	}
	return out
}

// DefaultFor returns the default choice of one change (SPEC §8): keep what exists
// in one input only; the newer version of what differs (A on a tie); for a
// deletion, drop when the deletion is newer than the last modification of the
// entry, else keep; A's settings.
func DefaultFor(c diff.Change) Choice {
	switch c.Kind {
	case diff.OnlyA, diff.OnlyB:
		return Keep
	case diff.Differs, diff.GroupDiffers:
		return newer(c)
	case diff.DeletedInA, diff.DeletedInB:
		survivor := diff.SideB
		if c.Kind == diff.DeletedInB {
			survivor = diff.SideA
		}
		if c.Deleted.After(c.Modified[survivor]) {
			return Drop
		}
		return Keep
	case diff.Settings:
		return TakeA
	}
	return Keep
}

// newer is take-b when B was modified strictly after A, else take-a.
func newer(c diff.Change) Choice {
	if c.Modified[diff.SideB].After(c.Modified[diff.SideA]) {
		return TakeB
	}
	return TakeA
}

// Filter selects the changes a rule is applied to: it gets the index and the
// change. A nil filter selects every change.
type Filter func(i int, c diff.Change) bool

// ApplyRule sets the choice of every selected change by the rule (SPEC §9).
func ApplyRule(p *Plan, rule Rule, filter Filter) {
	for i, c := range p.Changes {
		if filter != nil && !filter(i, c) {
			continue
		}
		p.Choices[i] = RuleChoice(rule, c)
	}
}

// RuleChoice returns the choice a rule gives to one change.
func RuleChoice(rule Rule, c diff.Change) Choice {
	switch rule {
	case PreferA:
		return prefer(c, diff.SideA)
	case PreferB:
		return prefer(c, diff.SideB)
	case Union:
		switch c.Kind {
		case diff.OnlyA, diff.OnlyB:
			return Keep
		case diff.Differs:
			return Both
		}
	}
	return DefaultFor(c)
}

// prefer is the choice that lets side s win.
func prefer(c diff.Change, s diff.Side) Choice {
	take := TakeA
	if s == diff.SideB {
		take = TakeB
	}
	switch c.Kind {
	case diff.OnlyA:
		return keepIf(s == diff.SideA)
	case diff.OnlyB:
		return keepIf(s == diff.SideB)
	case diff.Differs, diff.GroupDiffers, diff.Settings:
		return take
	case diff.DeletedInA:
		// A deleted the entry, B still has it: A's state is "gone".
		return keepIf(s == diff.SideB)
	case diff.DeletedInB:
		return keepIf(s == diff.SideA)
	}
	return DefaultFor(c)
}

func keepIf(b bool) Choice {
	if b {
		return Keep
	}
	return Drop
}
