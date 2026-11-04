package plan

import (
	"slices"
	"testing"
	"time"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func hours(n int) time.Time { return t0.Add(time.Duration(n) * time.Hour) }

// change makes a change of a kind with the modification times of A and B (in hours),
// and the deletion time.
func change(kind diff.Kind, a, b, deleted int) diff.Change {
	c := diff.Change{Kind: kind, Modified: [2]time.Time{hours(a), hours(b)}, Deleted: hours(deleted)}
	if kind == diff.Settings {
		c.Item = diff.ItemSettings
	}
	return c
}

func TestAllowed(t *testing.T) {
	tests := []struct {
		kind diff.Kind
		want []Choice
	}{
		{diff.OnlyA, []Choice{Keep, Drop}},
		{diff.OnlyB, []Choice{Keep, Drop}},
		{diff.Differs, []Choice{TakeA, TakeB, Both, Drop}},
		{diff.DeletedInA, []Choice{Keep, Drop}},
		{diff.DeletedInB, []Choice{Keep, Drop}},
		{diff.GroupDiffers, []Choice{TakeA, TakeB}},
		{diff.Settings, []Choice{TakeA, TakeB}},
		{diff.Kind(99), nil},
	}
	for _, tt := range tests {
		t.Run(tt.kind.String(), func(t *testing.T) {
			if got := Allowed(tt.kind); !slices.Equal(got, tt.want) {
				t.Errorf("Allowed = %v, want %v", got, tt.want)
			}
			for _, c := range []Choice{Keep, Drop, TakeA, TakeB, Both, "nope"} {
				if IsAllowed(tt.kind, c) != slices.Contains(tt.want, c) {
					t.Errorf("IsAllowed(%s) is wrong", c)
				}
			}
		})
	}
}

func TestDefaults(t *testing.T) {
	tests := []struct {
		name string
		c    diff.Change
		want Choice
	}{
		{"only-a is kept", change(diff.OnlyA, 1, 0, 0), Keep},
		{"only-b is kept", change(diff.OnlyB, 0, 1, 0), Keep},
		{"differs: A newer", change(diff.Differs, 5, 3, 0), TakeA},
		{"differs: B newer", change(diff.Differs, 3, 5, 0), TakeB},
		{"differs: a tie goes to A", change(diff.Differs, 4, 4, 0), TakeA},
		{"group-differs: B newer", change(diff.GroupDiffers, 1, 2, 0), TakeB},
		{"group-differs: a tie goes to A", change(diff.GroupDiffers, 2, 2, 0), TakeA},
		{"settings: A", change(diff.Settings, 1, 9, 0), TakeA},
		{"deleted-in-a: deletion newer than B's change", change(diff.DeletedInA, 0, 5, 6), Drop},
		{"deleted-in-a: B changed after the deletion", change(diff.DeletedInA, 0, 7, 6), Keep},
		{"deleted-in-a: same time keeps", change(diff.DeletedInA, 0, 6, 6), Keep},
		{"deleted-in-b: deletion newer than A's change", change(diff.DeletedInB, 5, 0, 6), Drop},
		{"deleted-in-b: A changed after the deletion", change(diff.DeletedInB, 7, 0, 6), Keep},
		{"deleted-in-b ignores A's time of the other side", change(diff.DeletedInB, 5, 99, 6), Drop},
		{"an unknown kind is kept", change(diff.Kind(99), 0, 0, 0), Keep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultFor(tt.c); got != tt.want {
				t.Errorf("DefaultFor = %s, want %s", got, tt.want)
			}
			if !IsAllowed(tt.c.Kind, DefaultFor(tt.c)) && tt.c.Kind != 99 {
				t.Errorf("the default is not allowed")
			}
		})
	}
}

func TestRules(t *testing.T) {
	// Each row is a kind with times where B is newer and the deletion is newer.
	tests := []struct {
		name               string
		c                  diff.Change
		newer, a, b, union Choice
	}{
		{"only-a", change(diff.OnlyA, 1, 0, 0), Keep, Keep, Drop, Keep},
		{"only-b", change(diff.OnlyB, 0, 1, 0), Keep, Drop, Keep, Keep},
		{"differs (B newer)", change(diff.Differs, 1, 2, 0), TakeB, TakeA, TakeB, Both},
		{"differs (A newer)", change(diff.Differs, 2, 1, 0), TakeA, TakeA, TakeB, Both},
		{"deleted-in-a (deletion newer)", change(diff.DeletedInA, 0, 1, 2), Drop, Drop, Keep, Drop},
		{"deleted-in-a (edit newer)", change(diff.DeletedInA, 0, 3, 2), Keep, Drop, Keep, Keep},
		{"deleted-in-b (deletion newer)", change(diff.DeletedInB, 1, 0, 2), Drop, Keep, Drop, Drop},
		{"deleted-in-b (edit newer)", change(diff.DeletedInB, 3, 0, 2), Keep, Keep, Drop, Keep},
		{"group-differs (B newer)", change(diff.GroupDiffers, 1, 2, 0), TakeB, TakeA, TakeB, TakeB},
		{"settings", change(diff.Settings, 1, 2, 0), TakeA, TakeA, TakeB, TakeA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for rule, want := range map[Rule]Choice{Newer: tt.newer, PreferA: tt.a, PreferB: tt.b, Union: tt.union} {
				got := RuleChoice(rule, tt.c)
				if got != want {
					t.Errorf("rule %s = %s, want %s", rule, got, want)
				}
				if !IsAllowed(tt.c.Kind, got) {
					t.Errorf("rule %s gives %s, which %s does not allow", rule, got, tt.c.Kind)
				}
			}
		})
	}
}

func TestApplyRuleWithFilter(t *testing.T) {
	changes := []diff.Change{
		change(diff.OnlyA, 1, 0, 0), change(diff.Differs, 1, 2, 0), change(diff.OnlyB, 0, 1, 0), change(diff.Differs, 3, 2, 0),
	}
	p := New(Input{Name: "a"}, Input{Name: "b"}, changes)
	if want := []Choice{Keep, TakeB, Keep, TakeA}; !slices.Equal(p.Choices, want) {
		t.Fatalf("defaults = %v, want %v", p.Choices, want)
	}
	// Only the changes the filter selects are set.
	ApplyRule(p, PreferA, func(i int, c diff.Change) bool { return c.Kind == diff.Differs })
	if want := []Choice{Keep, TakeA, Keep, TakeA}; !slices.Equal(p.Choices, want) {
		t.Errorf("after a filtered rule = %v, want %v", p.Choices, want)
	}
	ApplyRule(p, PreferB, nil)
	if want := []Choice{Drop, TakeB, Keep, TakeB}; !slices.Equal(p.Choices, want) {
		t.Errorf("after a rule for all = %v, want %v", p.Choices, want)
	}
	ApplyRule(p, Union, func(i int, _ diff.Change) bool { return i < 2 })
	if want := []Choice{Keep, Both, Keep, TakeB}; !slices.Equal(p.Choices, want) {
		t.Errorf("after union on the first two = %v, want %v", p.Choices, want)
	}
	ApplyRule(p, Newer, nil)
	if want := Default(changes); !slices.Equal(p.Choices, want) {
		t.Errorf("newer = %v, want the defaults %v", p.Choices, want)
	}
}

func TestSet(t *testing.T) {
	p := New(Input{}, Input{}, []diff.Change{change(diff.OnlyA, 1, 0, 0), change(diff.Differs, 1, 2, 0)})
	tests := []struct {
		i    int
		c    Choice
		ok   bool
		want Choice
	}{
		{0, Drop, true, Drop},
		{0, TakeA, false, Drop},
		{1, Both, true, Both},
		{1, Keep, false, Both},
		{2, Keep, false, ""},
		{-1, Keep, false, ""},
	}
	for _, tt := range tests {
		if got := p.Set(tt.i, tt.c); got != tt.ok {
			t.Errorf("Set(%d, %s) = %v, want %v", tt.i, tt.c, got, tt.ok)
		}
		if tt.i >= 0 && tt.i < len(p.Choices) && p.Choices[tt.i] != tt.want {
			t.Errorf("after Set(%d, %s) the choice is %s, want %s", tt.i, tt.c, p.Choices[tt.i], tt.want)
		}
	}
}

func TestParse(t *testing.T) {
	for _, r := range Rules {
		if got, ok := ParseRule(string(r)); !ok || got != r {
			t.Errorf("rule %s does not parse back", r)
		}
	}
	for _, c := range []Choice{Keep, Drop, TakeA, TakeB, Both} {
		if got, ok := ParseChoice(string(c)); !ok || got != c {
			t.Errorf("choice %s does not parse back", c)
		}
	}
	if _, ok := ParseRule("x"); ok {
		t.Error("parsed a bad rule")
	}
	if _, ok := ParseChoice("x"); ok {
		t.Error("parsed a bad choice")
	}
}

// Every rule gives an allowed choice to every change of a real comparison.
func TestRulesOnRealChanges(t *testing.T) {
	a := &model.Database{Root: &model.Group{UUID: model.UUID{15: 1}, Name: "Root"}}
	b := &model.Database{Root: &model.Group{UUID: model.UUID{15: 1}, Name: "Root2"}}
	b.Meta.DatabaseName = "B"
	changes := diff.Changes(a, b)
	if len(changes) == 0 {
		t.Fatal("no changes")
	}
	p := New(Input{}, Input{}, changes)
	for _, r := range Rules {
		ApplyRule(p, r, nil)
		for i, c := range p.Changes {
			if !IsAllowed(c.Kind, p.Choices[i]) {
				t.Errorf("rule %s: %s is not allowed for %s", r, p.Choices[i], c.Kind)
			}
		}
	}
}
