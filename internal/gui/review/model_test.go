package review

import (
	"slices"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

// pairModel is a model of the merge fixtures' nine changes:
//
//	0 only-a Email/Work mail        5 deleted-in-a Social/Old account
//	1 only-b Games (group)          6 deleted-in-b Social/Stale login
//	2 only-b Games/Old forum        7 group-differs Shopping
//	3 differs Banking/My bank       8 settings
//	4 differs Shopping/Store
func pairModel(t *testing.T) *Model {
	t.Helper()
	a, b := modeltest.MergePair()
	p := plan.New(plan.Input{Name: "a.kdbx"}, plan.Input{Name: "b.kdbx"}, diff.Changes(a, b))
	if len(p.Changes) != 9 {
		t.Fatalf("the fixture has %d changes, want 9", len(p.Changes))
	}
	return NewModel(p, nil)
}

func positions(m *Model) []int {
	out := make([]int, m.Len())
	for pos := range out {
		out[pos] = m.Index(pos)
	}
	return out
}

func TestModelFilter(t *testing.T) {
	tests := []struct {
		name  string
		setup func(m *Model)
		want  []int
	}{
		{"all", func(*Model) {}, []int{0, 1, 2, 3, 4, 5, 6, 7, 8}},
		{"one kind", func(m *Model) { m.SetKind(diff.Differs) }, []int{3, 4}},
		{"a kind with nothing", func(m *Model) { m.SetKind(diff.Differs); m.SetKind(diff.DeletedInA); m.SetQuery("zzz") }, nil},
		{"kind cleared", func(m *Model) { m.SetKind(diff.Differs); m.ClearKind() }, []int{0, 1, 2, 3, 4, 5, 6, 7, 8}},
		{"search the title", func(m *Model) { m.SetQuery("store") }, []int{4}},
		{"search ignores case and spaces around", func(m *Model) { m.SetQuery("  MY BANK ") }, []int{3}},
		{"search the group path", func(m *Model) { m.SetQuery("social") }, []int{5, 6}},
		{"search and kind", func(m *Model) { m.SetQuery("games"); m.SetKind(diff.OnlyB) }, []int{1, 2}},
		{"search finds no value", func(m *Model) { m.SetQuery("bank-password-a") }, nil},
		{"undecided only, nothing decided", func(m *Model) { m.SetUndecidedOnly(true) }, []int{0, 1, 2, 3, 4, 5, 6, 7, 8}},
		{"undecided only after a choice", func(m *Model) {
			m.SetUndecidedOnly(true)
			m.Set(3, plan.TakeB)
		}, []int{0, 1, 2, 4, 5, 6, 7, 8}},
		{"undecided off again", func(m *Model) {
			m.Set(3, plan.TakeB)
			m.SetUndecidedOnly(true)
			m.SetUndecidedOnly(false)
		}, []int{0, 1, 2, 3, 4, 5, 6, 7, 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := pairModel(t)
			tt.setup(m)
			if got := positions(m); !slices.Equal(got, tt.want) && (len(got) != 0 || len(tt.want) != 0) {
				t.Errorf("rows = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModelCounts(t *testing.T) {
	m := pairModel(t)
	want := map[diff.Kind]int{
		diff.OnlyA: 1, diff.OnlyB: 2, diff.Differs: 2, diff.DeletedInA: 1, diff.DeletedInB: 1, diff.GroupDiffers: 1, diff.Settings: 1,
	}
	for _, k := range diff.Kinds {
		if got := m.KindCount(k); got != want[k] {
			t.Errorf("KindCount(%v) = %d, want %d", k, got, want[k])
		}
	}
	if m.Total() != 9 || m.Undecided() != 9 {
		t.Errorf("Total, Undecided = %d, %d, want 9, 9", m.Total(), m.Undecided())
	}
	m.SetQuery("store") // the counts do not follow the filter
	if m.KindCount(diff.Differs) != 2 {
		t.Errorf("KindCount follows the search: %d", m.KindCount(diff.Differs))
	}
	m.Set(4, plan.Both)
	m.Set(4, plan.Drop) // deciding twice counts once
	if m.Undecided() != 8 {
		t.Errorf("Undecided = %d, want 8", m.Undecided())
	}
}

func TestModelSelection(t *testing.T) {
	type click struct {
		pos         int
		shift, ctrl bool
	}
	tests := []struct {
		name   string
		clicks []click
		want   []int // selected positions
		cursor int
	}{
		{"plain click", []click{{2, false, false}}, []int{2}, 2},
		{"second plain click replaces", []click{{2, false, false}, {5, false, false}}, []int{5}, 5},
		{"ctrl adds", []click{{2, false, false}, {5, false, true}}, []int{2, 5}, 5},
		{"ctrl removes", []click{{2, false, false}, {5, false, true}, {2, false, true}}, []int{5}, 2},
		{"shift selects the range down", []click{{2, false, false}, {5, true, false}}, []int{2, 3, 4, 5}, 5},
		{"shift selects the range up", []click{{5, false, false}, {2, true, false}}, []int{2, 3, 4, 5}, 2},
		{"shift keeps the anchor", []click{{2, false, false}, {5, true, false}, {3, true, false}}, []int{2, 3}, 3},
		{"shift without an anchor is a plain click", []click{{4, true, false}}, []int{4}, 4},
		{"ctrl+shift extends", []click{{0, false, false}, {2, false, true}, {5, true, true}}, []int{0, 2, 3, 4, 5}, 5},
		{"out of range is ignored", []click{{1, false, false}, {99, false, false}, {-1, false, false}}, []int{1}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := pairModel(t)
			for _, c := range tt.clicks {
				m.Click(c.pos, c.shift, c.ctrl)
			}
			if got := m.Selected(); !slices.Equal(got, tt.want) {
				t.Errorf("selected = %v, want %v", got, tt.want)
			}
			if m.Cursor() != tt.cursor {
				t.Errorf("cursor = %d, want %d", m.Cursor(), tt.cursor)
			}
			if m.SelectedCount() != len(tt.want) {
				t.Errorf("SelectedCount = %d, want %d", m.SelectedCount(), len(tt.want))
			}
		})
	}
}

func TestModelMove(t *testing.T) {
	tests := []struct {
		name  string
		steps func(m *Model)
		want  []int
		to    int
	}{
		{"down from nothing selects the first", func(m *Model) { m.Move(1, false) }, []int{0}, 0},
		{"up from nothing selects the last", func(m *Model) { m.Move(-1, false) }, []int{8}, 8},
		{"down moves the selection", func(m *Model) { m.Move(1, false); m.Move(1, false) }, []int{1}, 1},
		{"up stops at the first", func(m *Model) { m.Move(1, false); m.Move(-1, false); m.Move(-1, false) }, []int{0}, 0},
		{"down stops at the last", func(m *Model) { m.MoveTo(-1, false); m.Move(1, false) }, []int{8}, 8},
		{"shift extends", func(m *Model) { m.Move(1, false); m.Move(1, true); m.Move(1, true) }, []int{0, 1, 2}, 2},
		{"shift can shrink again", func(m *Model) { m.Move(1, false); m.Move(1, true); m.Move(1, true); m.Move(-1, true) }, []int{0, 1}, 1},
		{"home and end", func(m *Model) { m.MoveTo(0, false); m.MoveTo(-1, true) }, []int{0, 1, 2, 3, 4, 5, 6, 7, 8}, 8},
		{"select all", func(m *Model) { m.SetKind(diff.Differs); m.SelectAll() }, []int{0, 1}, 0},
		{"clear", func(m *Model) { m.SelectAll(); m.ClearSelection() }, nil, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := pairModel(t)
			tt.steps(m)
			if got := m.Selected(); !slices.Equal(got, tt.want) && (len(got) != 0 || len(tt.want) != 0) {
				t.Errorf("selected = %v, want %v", got, tt.want)
			}
			if m.Cursor() != tt.to {
				t.Errorf("cursor = %d, want %d", m.Cursor(), tt.to)
			}
		})
	}
}

func TestModelSelectionFollowsTheFilter(t *testing.T) {
	m := pairModel(t)
	m.Click(3, false, false) // Banking/My bank
	m.Click(4, false, true)  // Shopping/Store
	m.SetKind(diff.Differs)
	if got := m.Selected(); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("both differing entries stay selected: %v", got)
	}
	if m.Cursor() != 1 {
		t.Errorf("the cursor follows its row: %d, want 1", m.Cursor())
	}
	m.SetQuery("bank")
	if got := m.Selected(); !slices.Equal(got, []int{0}) || m.Cursor() != -1 {
		t.Errorf("selected = %v, cursor = %d; the hidden row must leave the selection and the cursor", got, m.Cursor())
	}
	if _, ok := m.Current(); !ok {
		t.Error("Current should name the one selected row")
	}
	m.ClearSelection()
	if _, ok := m.Current(); ok {
		t.Error("Current names a change although nothing is selected")
	}
}

func TestModelChoose(t *testing.T) {
	tests := []struct {
		name    string
		action  Action
		index   int
		want    plan.Choice
		skipped bool
	}{
		{"A on a pair takes A", ActionA, 3, plan.TakeA, false},
		{"B on a pair takes B", ActionB, 3, plan.TakeB, false},
		{"A on a group takes A", ActionA, 7, plan.TakeA, false},
		{"B on settings takes B", ActionB, 8, plan.TakeB, false},
		{"A on an entry only A has keeps it", ActionA, 0, plan.Keep, false},
		{"B on an entry only A has drops it", ActionB, 0, plan.Drop, false},
		{"A on an entry only B has drops it", ActionA, 2, plan.Drop, false},
		{"B on an entry only B has keeps it", ActionB, 2, plan.Keep, false},
		{"K on a pair keeps both", ActionKeep, 3, plan.Both, false},
		{"K on an entry that is in one input keeps it", ActionKeep, 0, plan.Keep, false},
		{"K on a deletion keeps the entry", ActionKeep, 5, plan.Keep, false},
		{"K on a group change has no meaning", ActionKeep, 7, "", true},
		{"K on settings has no meaning", ActionKeep, 8, "", true},
		{"D drops a pair", ActionDrop, 3, plan.Drop, false},
		{"D drops a deletion's entry", ActionDrop, 6, plan.Drop, false},
		{"D on a group change has no meaning", ActionDrop, 7, "", true},
		{"N gives the default of a pair", ActionNewer, 3, plan.DefaultFor(pairModel(t).plan.Changes[3]), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := pairModel(t)
			before := m.plan.Choices[tt.index]
			m.Click(tt.index, false, false)
			r := m.Choose(tt.action)
			if tt.skipped {
				if r.Skipped != 1 || r.Set != 0 || m.plan.Choices[tt.index] != before || m.decided[tt.index] {
					t.Errorf("result %+v, choice %q (was %q), decided %v; the row must stay as it was", r, m.plan.Choices[tt.index], before, m.decided[tt.index])
				}
				return
			}
			if r.Set != 1 || r.Skipped != 0 {
				t.Errorf("result = %+v, want one set", r)
			}
			if m.plan.Choices[tt.index] != tt.want {
				t.Errorf("choice = %q, want %q", m.plan.Choices[tt.index], tt.want)
			}
			if !m.decided[tt.index] || m.Undecided() != 8 {
				t.Errorf("the row must count as decided (undecided = %d)", m.Undecided())
			}
		})
	}
}

func TestModelChooseManyRows(t *testing.T) {
	m := pairModel(t)
	m.SelectAll()
	r := m.Choose(ActionDrop)
	// Group changes and settings cannot be dropped.
	if r.Set != 7 || r.Skipped != 2 {
		t.Errorf("result = %+v, want 7 set and 2 skipped", r)
	}
	for i, c := range m.plan.Changes {
		want := plan.Drop
		if !plan.IsAllowed(c.Kind, plan.Drop) {
			want = plan.DefaultFor(c)
		}
		if m.plan.Choices[i] != want {
			t.Errorf("change %d (%v): choice %q, want %q", i, c.Kind, m.plan.Choices[i], want)
		}
		if m.decided[i] == !plan.IsAllowed(c.Kind, plan.Drop) {
			t.Errorf("change %d (%v): decided = %v", i, c.Kind, m.decided[i])
		}
	}
	// Every choice that the model gives is one the kind allows (the plan would not be written otherwise).
	for _, a := range []Action{ActionA, ActionB, ActionKeep, ActionDrop, ActionNewer} {
		m := pairModel(t)
		m.SelectAll()
		m.Choose(a)
		for i, c := range m.plan.Changes {
			if !plan.IsAllowed(c.Kind, m.plan.Choices[i]) {
				t.Errorf("action %d gave %q to a %v", a, m.plan.Choices[i], c.Kind)
			}
		}
	}
}

func TestModelUndecidedFilterAdvances(t *testing.T) {
	m := pairModel(t)
	m.SetUndecidedOnly(true)
	m.Click(3, false, false) // Banking/My bank, index 3
	m.Choose(ActionB)
	if m.Len() != 8 {
		t.Fatalf("a decided row leaves the undecided list: %d rows", m.Len())
	}
	if got := m.Selected(); !slices.Equal(got, []int{3}) || m.Index(3) != 4 {
		t.Errorf("the next row (4) must slide under the cursor: selected %v, index %d", got, m.Index(3))
	}
	// At the end of the list the cursor stays on the last row.
	m.Click(m.Len()-1, false, false)
	m.Choose(ActionA)
	if got := m.Selected(); len(got) != 1 || got[0] != m.Len()-1 {
		t.Errorf("selected = %v with %d rows", got, m.Len())
	}
	for m.Len() > 0 {
		m.Click(0, false, false)
		m.Choose(ActionNewer)
	}
	if m.Undecided() != 0 || len(m.Selected()) != 0 {
		t.Errorf("undecided %d, selected %v after deciding everything", m.Undecided(), m.Selected())
	}
}

func TestModelApplyRule(t *testing.T) {
	tests := []struct {
		rule plan.Rule
		kind diff.Kind
		set  bool
	}{
		{plan.PreferA, diff.Differs, true},
		{plan.PreferB, diff.Differs, true},
		{plan.Union, diff.Differs, true},
		{plan.Newer, diff.Differs, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.rule), func(t *testing.T) {
			m := pairModel(t)
			m.SetKind(tt.kind) // the rule applies to the rows shown, not to the others
			m.Click(0, false, false)
			n := m.ApplyRule(tt.rule)
			if n != 2 {
				t.Errorf("ApplyRule set %d rows, want the 2 shown", n)
			}
			for i, c := range m.plan.Changes {
				want := plan.DefaultFor(c)
				shown := c.Kind == tt.kind
				if shown {
					want = plan.RuleChoice(tt.rule, c)
				}
				if m.plan.Choices[i] != want || m.decided[i] != shown {
					t.Errorf("change %d (%v): choice %q decided %v; want %q, %v", i, c.Kind, m.plan.Choices[i], m.decided[i], want, shown)
				}
			}
		})
	}
}

func TestModelReplace(t *testing.T) {
	m := pairModel(t)
	choices := slices.Clone(m.plan.Choices)
	choices[3] = plan.Drop
	m.Replace(choices)
	if m.plan.Choices[3] != plan.Drop || m.Undecided() != 0 {
		t.Errorf("choice %q, undecided %d; a loaded plan decides everything", m.plan.Choices[3], m.Undecided())
	}
}

func TestModelKeepsDecidedFromTheSession(t *testing.T) {
	m := pairModel(t)
	m.Set(2, plan.Drop)
	again := NewModel(m.Plan(), m.Decided())
	if again.Undecided() != 8 {
		t.Errorf("a new model of the same plan and decided slice: undecided = %d, want 8", again.Undecided())
	}
	if m.Set(3, plan.Keep) {
		t.Error("Keep is not a choice for a pair that differs")
	}
	if m.Undecided() != 8 {
		t.Errorf("a refused choice must not count as decided: %d", m.Undecided())
	}
}

func TestLabels(t *testing.T) {
	for _, k := range diff.Kinds {
		if KindLabel(k) == "" || KindLabel(k) == k.String() {
			t.Errorf("kind %v has no label", k)
		}
	}
	for _, c := range []plan.Choice{plan.Keep, plan.Drop, plan.TakeA, plan.TakeB, plan.Both} {
		if ChoiceLabel(c) == string(c) {
			t.Errorf("choice %q has no label", c)
		}
	}
	for _, r := range plan.Rules {
		if RuleLabel(r) == string(r) {
			t.Errorf("rule %q has no label", r)
		}
	}
	m := pairModel(t)
	tests := []struct {
		index int
		want  string
	}{
		{0, "Email / Work mail"}, {1, "Games (group)"}, {7, "Shopping (group)"}, {8, "Database settings"},
	}
	for _, tt := range tests {
		if got := Title(m.plan.Changes[tt.index]); got != tt.want {
			t.Errorf("Title(%d) = %q, want %q", tt.index, got, tt.want)
		}
	}
}
