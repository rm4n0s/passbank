package review

import (
	"slices"
	"strings"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

// Action is what a key of the Review screen does to the selected rows (SPEC §13).
type Action int

// The actions, in the order of their keys.
const (
	// ActionA lets A win: Take A where the kind has it, else what the rule "a" of
	// SPEC §9 gives (keep what only A has, drop what only B has).
	ActionA Action = iota
	// ActionB lets B win, the same way.
	ActionB
	// ActionKeep keeps an item that exists in one input only, or keeps both
	// entries of a pair that differs; other kinds have no such choice.
	ActionKeep
	// ActionDrop leaves the item out of the result.
	ActionDrop
	// ActionNewer sets the default of SPEC §8, which prefers the newer version.
	ActionNewer
)

// Result says what an action did.
type Result struct {
	// Set is the number of rows that got a choice.
	Set int
	// Skipped is the number of selected rows whose kind has no such choice.
	Skipped int
}

// Model is the state of the list of changes: the plan being edited, which changes
// the user has decided, the filter, the search, and the selection. It holds no
// widgets, so it is tested without a window. Every method runs on the UI thread.
type Model struct {
	plan    *plan.Plan
	decided []bool
	// search holds, for each change, its lower-case path and title.
	search []string
	counts map[diff.Kind]int
	// undecided is the number of changes that are not decided.
	undecided int

	kind          diff.Kind
	hasKind       bool
	undecidedOnly bool
	query         string

	// visible lists the indexes of the changes that pass the filter and the search.
	visible []int
	// selected is the set of selected changes, by index.
	selected map[int]struct{}
	// anchor and cursor are positions in visible, or -1.
	anchor, cursor int
}

// NewModel returns a model of the plan. decided says for each change whether the
// user has chosen already; the model changes it as the user chooses, so the caller
// keeps it with the plan. A nil or short slice is replaced by one that fits.
func NewModel(p *plan.Plan, decided []bool) *Model {
	if len(decided) != len(p.Changes) {
		decided = make([]bool, len(p.Changes))
	}
	m := &Model{plan: p, decided: decided, counts: map[diff.Kind]int{}, selected: map[int]struct{}{}, anchor: -1, cursor: -1}
	m.search = make([]string, len(p.Changes))
	for i, c := range p.Changes {
		m.search[i] = strings.ToLower(c.Path + "\x00" + c.Title)
		m.counts[c.Kind]++
		if !decided[i] {
			m.undecided++
		}
	}
	m.refilter()
	return m
}

// Plan is the plan the model edits.
func (m *Model) Plan() *plan.Plan { return m.plan }

// Decided is the slice that says which changes the user has chosen for.
func (m *Model) Decided() []bool { return m.decided }

// Total is the number of changes.
func (m *Model) Total() int { return len(m.plan.Changes) }

// KindCount is the number of changes of a kind, whatever the filter is.
func (m *Model) KindCount(k diff.Kind) int { return m.counts[k] }

// Undecided is the number of changes the user has not chosen for.
func (m *Model) Undecided() int { return m.undecided }

// Kind is the kind the list is filtered to; ok is false when it shows all kinds.
func (m *Model) Kind() (kind diff.Kind, ok bool) { return m.kind, m.hasKind }

// UndecidedOnly says whether the list shows only the changes not chosen for.
func (m *Model) UndecidedOnly() bool { return m.undecidedOnly }

// Query is the search text.
func (m *Model) Query() string { return m.query }

// SetKind filters the list to one kind.
func (m *Model) SetKind(k diff.Kind) {
	m.kind, m.hasKind = k, true
	m.refilter()
}

// ClearKind shows every kind again.
func (m *Model) ClearKind() {
	m.hasKind = false
	m.refilter()
}

// SetUndecidedOnly shows only the changes that the user has not chosen for (or all
// of them again).
func (m *Model) SetUndecidedOnly(on bool) {
	m.undecidedOnly = on
	m.refilter()
}

// SetQuery filters the list to the changes whose group path or title contains the
// text, ignoring case.
func (m *Model) SetQuery(q string) {
	m.query = q
	m.refilter()
}

// Len is the number of rows shown.
func (m *Model) Len() int { return len(m.visible) }

// Index is the index in the plan of the change shown at position pos.
func (m *Model) Index(pos int) int { return m.visible[pos] }

// Change is the change shown at position pos.
func (m *Model) Change(pos int) diff.Change { return m.plan.Changes[m.visible[pos]] }

// Choice is the choice of the change shown at position pos.
func (m *Model) Choice(pos int) plan.Choice { return m.plan.Choices[m.visible[pos]] }

// IsDecided says whether the user has chosen for the change shown at pos.
func (m *Model) IsDecided(pos int) bool { return m.decided[m.visible[pos]] }

// IsSelected says whether the row at position pos is selected.
func (m *Model) IsSelected(pos int) bool {
	_, ok := m.selected[m.visible[pos]]
	return ok
}

// Cursor is the position of the row that has the keyboard cursor, or -1.
func (m *Model) Cursor() int { return m.cursor }

// SelectedCount is the number of selected rows.
func (m *Model) SelectedCount() int { return len(m.selected) }

// Selected lists the positions of the selected rows, in list order.
func (m *Model) Selected() []int {
	var out []int
	for pos, i := range m.visible {
		if _, ok := m.selected[i]; ok {
			out = append(out, pos)
		}
	}
	return out
}

// Current is the change that the details show: the one under the cursor when it is
// selected, else the first selected one. ok is false when nothing is selected.
func (m *Model) Current() (index int, ok bool) {
	if m.cursor >= 0 && m.cursor < len(m.visible) && m.IsSelected(m.cursor) {
		return m.visible[m.cursor], true
	}
	if sel := m.Selected(); len(sel) > 0 {
		return m.visible[sel[0]], true
	}
	return 0, false
}

// Click selects as a click on the row at pos does: a plain click selects that row
// alone, Ctrl adds the row to the selection or removes it, and Shift selects the
// rows from the anchor (the last row clicked without Shift) to this one.
func (m *Model) Click(pos int, shift, ctrl bool) {
	if pos < 0 || pos >= len(m.visible) {
		return
	}
	switch {
	case shift && m.anchor >= 0:
		if !ctrl {
			m.clearSelection()
		}
		m.selectRange(m.anchor, pos)
	case ctrl:
		i := m.visible[pos]
		if _, ok := m.selected[i]; ok {
			delete(m.selected, i)
		} else {
			m.selected[i] = struct{}{}
		}
		m.anchor = pos
	default:
		m.clearSelection()
		m.selected[m.visible[pos]] = struct{}{}
		m.anchor = pos
	}
	m.cursor = pos
}

// Move moves the keyboard cursor by delta rows and selects the row it arrives at;
// with extend (the Shift key) the selection becomes the rows from the anchor to the
// cursor instead.
func (m *Model) Move(delta int, extend bool) {
	if len(m.visible) == 0 {
		return
	}
	pos := m.cursor + delta
	if m.cursor < 0 {
		pos = 0
		if delta < 0 {
			pos = len(m.visible) - 1
		}
	}
	pos = max(0, min(pos, len(m.visible)-1))
	if extend && m.anchor >= 0 {
		m.clearSelection()
		m.selectRange(m.anchor, pos)
		m.cursor = pos
		return
	}
	m.Click(pos, false, false)
}

// MoveTo moves the cursor to the first or the last row (to = 0 or to = -1).
func (m *Model) MoveTo(pos int, extend bool) {
	if len(m.visible) == 0 {
		return
	}
	if pos < 0 {
		pos = len(m.visible) - 1
	}
	pos = min(pos, len(m.visible)-1)
	if extend && m.anchor >= 0 {
		m.clearSelection()
		m.selectRange(m.anchor, pos)
		m.cursor = pos
		return
	}
	m.Click(pos, false, false)
}

// SelectAll selects every row shown.
func (m *Model) SelectAll() {
	for _, i := range m.visible {
		m.selected[i] = struct{}{}
	}
	if len(m.visible) > 0 {
		m.anchor = 0
		if m.cursor < 0 {
			m.cursor = 0
		}
	}
}

// ClearSelection selects nothing.
func (m *Model) ClearSelection() {
	m.clearSelection()
	m.anchor, m.cursor = -1, -1
}

func (m *Model) clearSelection() { clear(m.selected) }

func (m *Model) selectRange(from, to int) {
	if from > to {
		from, to = to, from
	}
	for pos := from; pos <= to; pos++ {
		m.selected[m.visible[pos]] = struct{}{}
	}
}

// Choose sets the choice of every selected row by the action. Rows whose kind has
// no such choice are left as they are and counted as skipped. Every row that gets a
// choice counts as decided, even when the choice is the one it had.
func (m *Model) Choose(a Action) Result {
	var r Result
	for _, pos := range m.Selected() {
		i := m.visible[pos]
		c, ok := actionChoice(a, m.plan.Changes[i])
		if !ok {
			r.Skipped++
			continue
		}
		m.set(i, c)
		r.Set++
	}
	m.afterChange()
	return r
}

// Set gives the change at index the choice, if its kind allows it. It reports
// whether it did.
func (m *Model) Set(index int, c plan.Choice) bool {
	if !m.plan.Set(index, c) {
		return false
	}
	m.markDecided(index)
	m.afterChange()
	return true
}

// ApplyRule sets the choice of every row shown, selected or not, by a rule of SPEC
// §9, and returns how many rows it set.
func (m *Model) ApplyRule(rule plan.Rule) int {
	for _, i := range m.visible {
		m.plan.Choices[i] = plan.RuleChoice(rule, m.plan.Changes[i])
		m.markDecided(i)
	}
	n := len(m.visible)
	m.afterChange()
	return n
}

// MarkAllDecided records that the user has chosen for every change; loading a plan
// file does so.
func (m *Model) MarkAllDecided() {
	for i := range m.decided {
		m.decided[i] = true
	}
	m.undecided = 0
	m.afterChange()
}

// Replace puts a loaded plan's choices into the plan being edited. The plan must
// be for the same changes.
func (m *Model) Replace(choices []plan.Choice) {
	copy(m.plan.Choices, choices)
	m.MarkAllDecided()
}

func (m *Model) set(i int, c plan.Choice) {
	m.plan.Choices[i] = c
	m.markDecided(i)
}

func (m *Model) markDecided(i int) {
	if !m.decided[i] {
		m.decided[i] = true
		m.undecided--
	}
}

// afterChange refilters, because a decided row leaves the "undecided" list, and
// keeps the cursor where it was so that the next row slides under it.
func (m *Model) afterChange() {
	if !m.undecidedOnly {
		return
	}
	pos := m.cursor
	m.refilter()
	if pos >= 0 && len(m.visible) > 0 {
		m.Click(min(pos, len(m.visible)-1), false, false)
	}
}

// actionChoice is the choice an action gives to a change; ok is false when the
// change's kind has none.
func actionChoice(a Action, c diff.Change) (plan.Choice, bool) {
	switch a {
	case ActionA:
		if plan.IsAllowed(c.Kind, plan.TakeA) {
			return plan.TakeA, true
		}
		return plan.RuleChoice(plan.PreferA, c), true
	case ActionB:
		if plan.IsAllowed(c.Kind, plan.TakeB) {
			return plan.TakeB, true
		}
		return plan.RuleChoice(plan.PreferB, c), true
	case ActionKeep:
		for _, ch := range []plan.Choice{plan.Keep, plan.Both} {
			if plan.IsAllowed(c.Kind, ch) {
				return ch, true
			}
		}
		return "", false
	case ActionDrop:
		if plan.IsAllowed(c.Kind, plan.Drop) {
			return plan.Drop, true
		}
		return "", false
	case ActionNewer:
		return plan.RuleChoice(plan.Newer, c), true
	}
	return "", false
}

// refilter rebuilds the rows shown. The selection keeps the rows that are still
// shown; the cursor and the anchor follow their rows or are lost.
func (m *Model) refilter() {
	q := strings.ToLower(strings.TrimSpace(m.query))
	cursorIdx, anchorIdx := -1, -1
	if m.cursor >= 0 && m.cursor < len(m.visible) {
		cursorIdx = m.visible[m.cursor]
	}
	if m.anchor >= 0 && m.anchor < len(m.visible) {
		anchorIdx = m.visible[m.anchor]
	}
	m.visible = m.visible[:0]
	for i, c := range m.plan.Changes {
		if m.hasKind && c.Kind != m.kind {
			continue
		}
		if m.undecidedOnly && m.decided[i] {
			continue
		}
		if q != "" && !strings.Contains(m.search[i], q) {
			continue
		}
		m.visible = append(m.visible, i)
	}
	for i := range m.selected {
		if _, ok := slices.BinarySearch(m.visible, i); !ok {
			delete(m.selected, i)
		}
	}
	m.cursor, m.anchor = -1, -1
	if p, ok := slices.BinarySearch(m.visible, cursorIdx); ok {
		m.cursor = p
	}
	if p, ok := slices.BinarySearch(m.visible, anchorIdx); ok {
		m.anchor = p
	}
}

// KindLabel is the name of a kind of change on the screen.
func KindLabel(k diff.Kind) string {
	switch k {
	case diff.OnlyA:
		return "Only in A"
	case diff.OnlyB:
		return "Only in B"
	case diff.Differs:
		return "Differs"
	case diff.DeletedInA:
		return "Deleted in A"
	case diff.DeletedInB:
		return "Deleted in B"
	case diff.GroupDiffers:
		return "Group differs"
	case diff.Settings:
		return "Settings"
	}
	return k.String()
}

// ChoiceLabel is the name of a choice on the screen.
func ChoiceLabel(c plan.Choice) string {
	switch c {
	case plan.Keep:
		return "Keep"
	case plan.Drop:
		return "Drop"
	case plan.TakeA:
		return "Take A"
	case plan.TakeB:
		return "Take B"
	case plan.Both:
		return "Keep both"
	}
	return string(c)
}

// RuleLabel is the name of a rule of SPEC §9 on the screen.
func RuleLabel(r plan.Rule) string {
	switch r {
	case plan.Newer:
		return "Newer wins"
	case plan.PreferA:
		return "A wins"
	case plan.PreferB:
		return "B wins"
	case plan.Union:
		return "Keep everything"
	}
	return string(r)
}

// Title is how a row names its change: the group path and the title, or what the
// change is about when it has none.
func Title(c diff.Change) string {
	switch c.Item {
	case diff.ItemSettings:
		return "Database settings"
	case diff.ItemGroup:
		return join(c.Path, c.Title) + " (group)"
	}
	return join(c.Path, c.Title)
}

func join(path, title string) string {
	if path == "" {
		return title
	}
	return path + " / " + title
}
