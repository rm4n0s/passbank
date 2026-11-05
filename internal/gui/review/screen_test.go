package review

import (
	"slices"
	"testing"
	"time"

	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/services"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/plan"
	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/session"
	"github.com/rm4n0s/passbank/internal/gui/testutil"
)

// harness is the Review screen in a window, with a session of two databases.
type harness struct {
	t          *testing.T
	ts         *gluttertest.Tester
	fd         *gluttertest.FakeDesktop
	sess       *session.Session
	back, next int
	theme      kit.Theme
}

func sessionOf(a, b *model.Database) *session.Session {
	s := session.New()
	s.A = &session.Source{Path: "/fixtures/a.kdbx", DB: a}
	s.B = &session.Source{Path: "/fixtures/b.kdbx", DB: b}
	return s
}

func newHarness(t *testing.T, theme kit.Theme, sess *session.Session) *harness {
	t.Helper()
	h := &harness{t: t, sess: sess, theme: theme}
	t.Cleanup(sess.Wipe)
	h.ts = testutil.New(t)
	h.fd = h.ts.FakeDesktop()
	h.ts.SetViewSize(ui.Size{Width: 1200, Height: 760})
	h.show()
	return h
}

// show builds the screen and waits for the comparison to finish.
func (h *harness) show() {
	h.t.Helper()
	h.ts.PumpWidget(widgets.NewDirectionality(ui.TextDirectionLTR,
		kit.NewRoot(h.theme, NewScreen(h.sess, func() { h.back++ }, func() { h.next++ }))))
	h.ts.PumpAndSettle()
	h.fd.WaitFor("the comparison", func() bool {
		h.ts.PumpAndSettle()
		return h.model() != nil
	})
}

func (h *harness) state() *screenState {
	return gluttertest.StateOf[*screenState](h.ts, gluttertest.ByType[*Screen]())
}

func (h *harness) model() *Model { return h.state().Model() }

func (h *harness) tapRow(text string) {
	h.t.Helper()
	h.ts.Tap(gluttertest.ByText(text))
	h.ts.PumpAndSettle()
}

func (h *harness) key(k services.LogicalKeyboardKey) {
	h.t.Helper()
	h.ts.SendKeyEvent(k)
	h.ts.PumpAndSettle()
}

func (h *harness) selected() []int { return h.model().Selected() }

func (h *harness) choice(index int) plan.Choice { return h.model().Plan().Choices[index] }

func TestScreenListsTheChanges(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	for _, want := range []string{
		"Email / Work mail", "Games (group)", "Games / Old forum", "Banking / My bank", "Shopping / Store",
		"Social / Old account", "Social / Stale login", "Shopping (group)", "Database settings",
		"All 9", "Only in A 1", "Only in B 2", "Differs 2", "Deleted in A 1", "Deleted in B 1", "Group differs 1", "Settings 1", "Undecided 9",
		"A: a.kdbx    B: b.kdbx",
	} {
		if !h.ts.Exists(gluttertest.ByText(want)) {
			t.Errorf("the screen does not show %q", want)
		}
	}
	if h.sess.Plan == nil || len(h.sess.Plan.Changes) != 9 || len(h.sess.Decided) != 9 {
		t.Fatal("the comparison did not leave a plan in the session")
	}
	if !h.ts.Exists(gluttertest.ByText("Select a change to see it.")) {
		t.Error("the details do not invite a selection")
	}
}

func TestScreenWithoutDatabases(t *testing.T) {
	h := &harness{t: t, sess: session.New(), theme: kit.LightTheme()}
	h.ts = testutil.New(t)
	h.ts.SetViewSize(ui.Size{Width: 800, Height: 400})
	h.ts.PumpWidget(widgets.NewDirectionality(ui.TextDirectionLTR, kit.NewRoot(h.theme, NewScreen(h.sess, func() {}, func() {}))))
	h.ts.PumpAndSettle()
	if !h.ts.Exists(gluttertest.ByText("Unlock both databases first.")) {
		t.Error("no hint that the databases are missing")
	}
	var next *kit.Button
	for _, e := range h.ts.Elements(gluttertest.ByType[*kit.Button]()) {
		if b := e.Widget().(*kit.Button); b.Label() == "Next: write the new database" {
			next = b
		}
	}
	if next == nil {
		t.Fatal("no Next button")
	}
	if next.Enabled() {
		t.Error("Next is enabled without a comparison")
	}
}

func TestScreenSelectionByMouse(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))

	h.tapRow("Banking / My bank")
	if got := h.selected(); !slices.Equal(got, []int{3}) {
		t.Fatalf("a click selected %v", got)
	}
	if !h.ts.Exists(gluttertest.ByText("Differs in")) || !h.ts.Exists(gluttertest.ByText("password, url")) {
		t.Error("the details do not show what differs")
	}

	h.ts.SendKeyDownEvent(services.LogicalKeyboardKeyShiftLeft)
	h.tapRow("Social / Old account")
	h.ts.SendKeyUpEvent(services.LogicalKeyboardKeyShiftLeft)
	if got := h.selected(); !slices.Equal(got, []int{3, 4, 5}) {
		t.Errorf("Shift+click selected %v, want 3, 4, 5", got)
	}

	h.ts.SendKeyDownEvent(services.LogicalKeyboardKeyControlLeft)
	h.tapRow("Email / Work mail")
	h.tapRow("Shopping / Store")
	h.ts.SendKeyUpEvent(services.LogicalKeyboardKeyControlLeft)
	if got := h.selected(); !slices.Equal(got, []int{0, 3, 5}) {
		t.Errorf("Ctrl+click selected %v, want 0, 3, 5", got)
	}
	if !h.ts.Exists(gluttertest.ByText("3 changes")) {
		t.Error("the details do not say that 3 are selected")
	}
}

func TestScreenKeys(t *testing.T) {
	tests := []struct {
		name  string
		row   string
		index int
		key   services.LogicalKeyboardKey
		want  plan.Choice
	}{
		{"A takes A on a pair", "Banking / My bank", 3, services.LogicalKeyboardKeyKeyA, plan.TakeA},
		{"B takes B on a pair", "Banking / My bank", 3, services.LogicalKeyboardKeyKeyB, plan.TakeB},
		{"K keeps both of a pair", "Shopping / Store", 4, services.LogicalKeyboardKeyKeyK, plan.Both},
		{"K keeps an entry that is in one input", "Games / Old forum", 2, services.LogicalKeyboardKeyKeyK, plan.Keep},
		{"D drops", "Email / Work mail", 0, services.LogicalKeyboardKeyKeyD, plan.Drop},
		{"N gives the newer", "Banking / My bank", 3, services.LogicalKeyboardKeyKeyN, plan.DefaultFor(mustChange(t, 3))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := modeltest.MergePair()
			h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
			h.tapRow(tt.row)
			h.key(tt.key)
			if got := h.choice(tt.index); got != tt.want {
				t.Errorf("choice = %q, want %q", got, tt.want)
			}
			if !h.sess.Decided[tt.index] || h.model().Undecided() != 8 {
				t.Errorf("the row must count as decided, undecided = %d", h.model().Undecided())
			}
		})
	}
}

// mustChange is change i of the merge fixtures.
func mustChange(t *testing.T, i int) diff.Change {
	t.Helper()
	a, b := modeltest.MergePair()
	return planOf(a, b).Changes[i]
}

func planOf(a, b *model.Database) *plan.Plan {
	return plan.New(plan.Input{Name: "a.kdbx"}, plan.Input{Name: "b.kdbx"}, diff.Changes(a, b))
}

func TestScreenKeysOnManyRows(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	h.tapRow("Banking / My bank")
	h.ts.SendKeyDownEvent(services.LogicalKeyboardKeyShiftLeft)
	h.tapRow("Shopping / Store")
	h.ts.SendKeyUpEvent(services.LogicalKeyboardKeyShiftLeft)
	h.key(services.LogicalKeyboardKeyKeyB)
	if h.choice(3) != plan.TakeB || h.choice(4) != plan.TakeB {
		t.Errorf("choices = %q, %q; the key works on every selected row", h.choice(3), h.choice(4))
	}
	if h.choice(2) == plan.TakeB || h.choice(5) == plan.TakeB {
		t.Error("the key reached a row that was not selected")
	}
}

func TestScreenArrowKeys(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	h.tapRow("Banking / My bank")
	h.key(services.LogicalKeyboardKeyArrowDown)
	if got := h.selected(); !slices.Equal(got, []int{4}) {
		t.Errorf("Down selected %v, want 4", got)
	}
	h.key(services.LogicalKeyboardKeyArrowUp)
	h.key(services.LogicalKeyboardKeyArrowUp)
	if got := h.selected(); !slices.Equal(got, []int{2}) {
		t.Errorf("Up, Up selected %v, want 2", got)
	}
	h.ts.SendKeyDownEvent(services.LogicalKeyboardKeyShiftLeft)
	h.key(services.LogicalKeyboardKeyArrowDown)
	h.key(services.LogicalKeyboardKeyArrowDown)
	h.ts.SendKeyUpEvent(services.LogicalKeyboardKeyShiftLeft)
	if got := h.selected(); !slices.Equal(got, []int{2, 3, 4}) {
		t.Errorf("Shift+Down selected %v, want 2, 3, 4", got)
	}
	h.key(services.LogicalKeyboardKeyEnd)
	if got := h.selected(); !slices.Equal(got, []int{8}) {
		t.Errorf("End selected %v, want 8", got)
	}
	h.key(services.LogicalKeyboardKeyHome)
	if got := h.selected(); !slices.Equal(got, []int{0}) {
		t.Errorf("Home selected %v, want 0", got)
	}
	h.ts.SendKeyDownEvent(services.LogicalKeyboardKeyControlLeft)
	h.key(services.LogicalKeyboardKeyKeyA)
	h.ts.SendKeyUpEvent(services.LogicalKeyboardKeyControlLeft)
	if got := len(h.selected()); got != 9 {
		t.Errorf("Ctrl+A selected %d rows, want 9", got)
	}
	h.key(services.LogicalKeyboardKeyEscape)
	if got := len(h.selected()); got != 0 {
		t.Errorf("Escape left %d rows selected", got)
	}
}

func TestScreenFilterAndSearch(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))

	h.tapRow("Differs 2")
	if got := positions(h.model()); !slices.Equal(got, []int{3, 4}) {
		t.Errorf("the Differs chip shows %v", got)
	}
	if h.ts.Exists(gluttertest.ByText("Email / Work mail")) {
		t.Error("a row of another kind is still shown")
	}
	h.tapRow("Differs 2") // again: off
	if h.model().Len() != 9 {
		t.Errorf("the chip did not switch off: %d rows", h.model().Len())
	}

	h.ts.EnterText(gluttertest.ByType[*widgets.EditableText](), "social")
	h.ts.PumpAndSettle()
	if got := positions(h.model()); !slices.Equal(got, []int{5, 6}) {
		t.Errorf("the search for social shows %v", got)
	}
	h.ts.EnterText(gluttertest.ByType[*widgets.EditableText](), "no such thing")
	h.ts.PumpAndSettle()
	if !h.ts.Exists(gluttertest.ByText("No change matches.")) {
		t.Error("an empty result does not say so")
	}
	h.ts.EnterText(gluttertest.ByType[*widgets.EditableText](), "")
	h.ts.PumpAndSettle()
	h.tapRow("All 9")

	// Undecided only: a decided row leaves the list.
	h.tapRow("Banking / My bank")
	h.key(services.LogicalKeyboardKeyKeyB)
	h.tapRow("Undecided 8")
	if h.model().Len() != 8 || h.ts.Exists(gluttertest.ByText("Banking / My bank")) {
		t.Errorf("the undecided list shows %d rows", h.model().Len())
	}
	// The next row slides under the cursor, so a run of keys goes down the list.
	h.tapRow("Email / Work mail")
	h.key(services.LogicalKeyboardKeyKeyD)
	h.key(services.LogicalKeyboardKeyKeyD)
	if h.choice(0) != plan.Drop || h.choice(1) != plan.Drop {
		t.Errorf("two D keys gave %q and %q to the first two rows", h.choice(0), h.choice(1))
	}
}

func TestScreenKeysInTheSearchFieldDoNotChooseRows(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	h.tapRow("Banking / My bank")
	before := slices.Clone(h.sess.Plan.Choices)
	h.ts.EnterText(gluttertest.ByType[*widgets.EditableText](), "dab")
	h.ts.PumpAndSettle()
	if !slices.Equal(h.sess.Plan.Choices, before) {
		t.Error("typing in the search field changed a choice")
	}
}

func TestScreenBulkRules(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	h.tapRow("Differs 2")
	h.tapRow("Keep everything")
	if h.choice(3) != plan.Both || h.choice(4) != plan.Both {
		t.Errorf("Keep everything gave %q, %q to the pairs", h.choice(3), h.choice(4))
	}
	if h.sess.Decided[0] || h.choice(0) != plan.DefaultFor(h.sess.Plan.Changes[0]) {
		t.Error("a rule changed a row that was not shown")
	}
	h.tapRow("A wins")
	if h.choice(3) != plan.TakeA || h.choice(4) != plan.TakeA {
		t.Errorf("A wins gave %q, %q", h.choice(3), h.choice(4))
	}
	h.tapRow("B wins")
	if h.choice(3) != plan.TakeB {
		t.Errorf("B wins gave %q", h.choice(3))
	}
	h.tapRow("Newer wins")
	if h.choice(3) != plan.DefaultFor(h.sess.Plan.Changes[3]) {
		t.Errorf("Newer wins gave %q", h.choice(3))
	}
	if h.model().Undecided() != 7 {
		t.Errorf("undecided = %d, want 7 after the rules decided 2 rows", h.model().Undecided())
	}
}

func TestScreenKeepsTheChoicesWhenItComesBack(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	h.tapRow("Banking / My bank")
	h.key(services.LogicalKeyboardKeyKeyA)
	h.ts.PumpWidget(widgets.NewSizedBox(nil)) // the Write screen
	h.ts.PumpAndSettle()
	h.show()
	if h.choice(3) != plan.TakeA || !h.sess.Decided[3] || h.model().Undecided() != 8 {
		t.Errorf("choice %q, decided %v, undecided %d after coming back", h.choice(3), h.sess.Decided[3], h.model().Undecided())
	}
}

func TestScreenButtons(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	h.tapRow("Back")
	h.tapRow("Next: write the new database")
	if h.back != 1 || h.next != 1 {
		t.Errorf("back %d, next %d", h.back, h.next)
	}
}

func TestScreenEmpty(t *testing.T) {
	a, _ := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, a.Clone()))
	if !h.ts.Exists(gluttertest.ByText("The two databases hold the same.")) {
		t.Error("two equal databases are not reported as equal")
	}
	if h.model().Len() != 0 || len(h.sess.Plan.Changes) != 0 {
		t.Errorf("%d rows for equal databases", h.model().Len())
	}
}

// The list builds the rows in view only, so 10,000 changes cost no more to show
// than a few. The bound is generous; docs/IMPLEMENTED.md records the real time.
func TestScreenWithTenThousandChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	a, b := modeltest.LargePair(50000)
	changes := planOf(a, b).Changes
	if len(changes) < 10000 {
		t.Fatalf("the large pair has %d changes, want at least 10,000", len(changes))
	}
	sess := sessionOf(a, b)
	start := time.Now()
	h := newHarness(t, kit.LightTheme(), sess)
	shown := time.Since(start)
	rows := len(h.ts.Elements(gluttertest.ByType[*kit.ListRow]()))
	if rows == 0 || rows > 60 {
		t.Errorf("%d rows are built for %d changes; only the rows in view should be", rows, h.model().Len())
	}

	start = time.Now()
	h.tapRow(h.rowText(0))
	h.key(services.LogicalKeyboardKeyEnd)
	end := time.Since(start)

	start = time.Now()
	h.ts.EnterText(gluttertest.ByType[*widgets.EditableText](), "Entry 39")
	h.ts.PumpAndSettle()
	search := time.Since(start)

	start = time.Now()
	h.model().ClearSelection()
	h.model().SetQuery("")
	h.tapRow("Newer wins")
	rule := time.Since(start)

	t.Logf("%d changes: first screen with the comparison %v, End key %v, search %v, a rule on all rows %v", h.model().Total(), shown, end, search, rule)
	for name, d := range map[string]time.Duration{"first screen": shown, "End key": end, "search": search, "rule": rule} {
		if d > 5*time.Second {
			t.Errorf("%s took %v", name, d)
		}
	}
}

// rowText is the text of the first row's title.
func (h *harness) rowText(pos int) string { return Title(h.model().Change(pos)) }

func TestScreenGolden(t *testing.T) {
	for _, th := range []struct {
		name  string
		theme kit.Theme
	}{{"light", kit.LightTheme()}, {"dark", kit.DarkTheme()}} {
		t.Run(th.name, func(t *testing.T) {
			a, b := modeltest.MergePair()
			h := newHarness(t, th.theme, sessionOf(a, b))
			// Mixed choices: some decided, some at their default.
			h.tapRow("Banking / My bank")
			h.key(services.LogicalKeyboardKeyKeyB)
			h.tapRow("Shopping / Store")
			h.key(services.LogicalKeyboardKeyKeyK)
			h.tapRow("Games / Old forum")
			h.key(services.LogicalKeyboardKeyKeyD)
			h.ts.SendKeyDownEvent(services.LogicalKeyboardKeyShiftLeft)
			h.tapRow("Social / Stale login")
			h.ts.SendKeyUpEvent(services.LogicalKeyboardKeyShiftLeft)
			h.ts.MatchesGolden("review-list-" + th.name)
			if th.name == "light" {
				testutil.Proof(t, h.ts, "8.3-review-list")
			}
		})
	}
}
