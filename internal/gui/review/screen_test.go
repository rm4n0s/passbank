package review

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/services"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/merge"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/plan"
	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/session"
	"github.com/rm4n0s/passbank/internal/gui/testutil"
)

// assertRoute fails the test unless err is an *errors.Error whose route contains route.
func assertRoute(t *testing.T, err error, route string) {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T: %v", err, err)
	}
	if !appErr.HasRoute(route) {
		t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), route)
	}
}

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

// tapRow taps the row of the list with the text, or else whatever has the text (a
// chip, a button). The details repeat the title of the selected row, so the list is
// searched first.
func (h *harness) tapRow(text string) {
	h.t.Helper()
	f := gluttertest.Descendant(gluttertest.ByType[*kit.ListRow](), gluttertest.ByText(text))
	if !h.ts.Exists(f) {
		f = gluttertest.ByText(text)
	}
	h.ts.Tap(f)
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
	for _, want := range []string{"https://bank.example.com", "https://bank.example.org", kit.Masked} {
		if !h.ts.Exists(gluttertest.ByText(want)) {
			t.Errorf("the details do not show %q", want)
		}
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
	if !h.ts.Exists(gluttertest.ByText("Only in A; 3 changes are selected, the buttons set them all")) {
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

// button finds the button with the label.
func (h *harness) button(label string) *kit.Button {
	h.t.Helper()
	for _, e := range h.ts.Elements(gluttertest.ByType[*kit.Button]()) {
		if b := e.Widget().(*kit.Button); b.Label() == label {
			return b
		}
	}
	h.t.Fatalf("no button %q", label)
	return nil
}

func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	h.fd.WaitFor(what, func() bool {
		h.ts.PumpAndSettle()
		return cond()
	})
}

func TestScreenRevealResetsWhenTheSelectionChanges(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	h.tapRow("Banking / My bank")
	if h.ts.Exists(gluttertest.ByText("bank-password-a")) || h.ts.Exists(gluttertest.ByText("bank-password-b")) {
		t.Fatal("the passwords are shown before anyone asked")
	}
	if !h.ts.Exists(gluttertest.ByText(kit.Masked)) {
		t.Fatal("the passwords are not masked")
	}
	h.ts.Tap(gluttertest.ByText("Show"))
	h.ts.PumpAndSettle()
	if !h.ts.Exists(gluttertest.ByText("bank-password-a")) || !h.ts.Exists(gluttertest.ByText("bank-password-b")) {
		t.Fatal("Show did not reveal the passwords")
	}
	h.tapRow("Shopping / Store")
	if h.ts.Exists(gluttertest.ByText("store-password")) {
		t.Error("the password of the next change was revealed")
	}
	h.tapRow("Banking / My bank")
	if h.ts.Exists(gluttertest.ByText("bank-password-a")) {
		t.Error("the reveal came back with the selection")
	}
	// A choice made on the same change keeps the reveal.
	h.ts.Tap(gluttertest.ByText("Show"))
	h.ts.PumpAndSettle()
	h.key(services.LogicalKeyboardKeyKeyA)
	if !h.ts.Exists(gluttertest.ByText("bank-password-a")) {
		t.Error("choosing hid the password again")
	}
}

func TestScreenChoiceButtons(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	enabled := func() map[string]bool {
		out := map[string]bool{}
		for _, label := range []string{"Take A", "Take B", "Keep both", "Keep", "Drop"} {
			out[label] = h.segmentEnabled(label)
		}
		return out
	}
	tests := []struct {
		row  string
		want map[string]bool
	}{
		{"Email / Work mail", map[string]bool{"Take A": false, "Take B": false, "Keep both": false, "Keep": true, "Drop": true}},
		{"Banking / My bank", map[string]bool{"Take A": true, "Take B": true, "Keep both": true, "Keep": false, "Drop": true}},
		{"Shopping (group)", map[string]bool{"Take A": true, "Take B": true, "Keep both": false, "Keep": false, "Drop": false}},
		{"Database settings", map[string]bool{"Take A": true, "Take B": true, "Keep both": false, "Keep": false, "Drop": false}},
		{"Social / Old account", map[string]bool{"Take A": false, "Take B": false, "Keep both": false, "Keep": true, "Drop": true}},
	}
	for _, tt := range tests {
		h.tapRow(tt.row)
		got := enabled()
		for label, want := range tt.want {
			if got[label] != want {
				t.Errorf("%s: button %q enabled = %v, want %v", tt.row, label, got[label], want)
			}
		}
	}

	h.tapRow("Banking / My bank")
	h.pressChoice("Take B")
	if h.choice(3) != plan.TakeB || !h.sess.Decided[3] {
		t.Errorf("the Take B button gave %q", h.choice(3))
	}
	h.pressChoice("Keep both")
	if h.choice(3) != plan.Both {
		t.Errorf("the Keep both button gave %q", h.choice(3))
	}
	// A disabled button does nothing.
	h.pressChoice("Keep")
	if h.choice(3) != plan.Both {
		t.Errorf("a disabled button set %q", h.choice(3))
	}
	// With several rows selected, a button sets those that allow it.
	h.tapRow("Banking / My bank")
	h.ts.SendKeyDownEvent(services.LogicalKeyboardKeyShiftLeft)
	h.tapRow("Shopping / Store")
	h.ts.SendKeyUpEvent(services.LogicalKeyboardKeyShiftLeft)
	h.pressChoice("Drop")
	if h.choice(3) != plan.Drop || h.choice(4) != plan.Drop {
		t.Errorf("Drop gave %q and %q", h.choice(3), h.choice(4))
	}
}

// segmentEnabled says whether the choice button with the label can be used.
func (h *harness) segmentEnabled(label string) bool {
	h.t.Helper()
	for _, e := range h.ts.Elements(gluttertest.ByType[*kit.Segmented]()) {
		if seg := e.Widget().(*kit.Segmented); seg.Label() == "Choice" {
			for _, sg := range seg.Segments() {
				if sg.Label == label {
					return !sg.Disabled
				}
			}
		}
	}
	h.t.Fatal("no choice buttons")
	return false
}

// pressChoice presses the choice button with the label (not the choice in a row).
func (h *harness) pressChoice(label string) {
	h.t.Helper()
	h.ts.Tap(gluttertest.Descendant(gluttertest.ByType[*kit.Segmented](), gluttertest.ByText(label)))
	h.ts.PumpAndSettle()
}

// planDialog presses the button and answers the file dialog it opens with path.
func (h *harness) planDialog(button, path string) {
	h.t.Helper()
	h.ts.Tap(gluttertest.ByText(button))
	h.ts.PumpAndSettle()
	h.waitFor("the file dialog", func() bool { return h.fd.PendingFileChoosers() == 1 })
	h.fd.AnswerFileChooser(path)
	h.waitFor("the dialog to close", func() bool { return h.fd.PendingFileChoosers() == 0 && !h.state().busy })
}

func (h *harness) noteText() string {
	if n := h.state().note; n != nil {
		return n.text
	}
	return ""
}

func TestScreenSavesAndLoadsAPlan(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	h.tapRow("Banking / My bank")
	h.key(services.LogicalKeyboardKeyKeyB)
	h.tapRow("Email / Work mail")
	h.key(services.LogicalKeyboardKeyKeyD)

	path := filepath.Join(t.TempDir(), "plan.txt")
	h.planDialog("Save plan…", path)
	if want := "Saved the plan to plan.txt."; h.noteText() != want {
		t.Fatalf("note = %q, want %q", h.noteText(), want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), plan.FormatLine) {
		t.Errorf("the plan file starts with %q", firstLine(string(data)))
	}
	for _, secret := range []string{"bank-password-a", "bank-password-b", "pw-work-mail"} {
		if strings.Contains(string(data), secret) {
			t.Errorf("the plan file holds the secret %q", secret)
		}
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}

	// Change the choices, then load the file: the saved choices come back, all decided.
	h.tapRow("Banking / My bank")
	h.key(services.LogicalKeyboardKeyKeyA)
	h.tapRow("Email / Work mail")
	h.key(services.LogicalKeyboardKeyKeyK)
	h.planDialog("Load plan…", path)
	if h.choice(3) != plan.TakeB || h.choice(0) != plan.Drop {
		t.Errorf("after loading: choices %q, %q; want take-b, drop", h.choice(3), h.choice(0))
	}
	if h.model().Undecided() != 0 {
		t.Errorf("undecided = %d; a loaded plan decides everything", h.model().Undecided())
	}
	if want := "Loaded the choices of plan.txt."; h.noteText() != want {
		t.Errorf("note = %q, want %q", h.noteText(), want)
	}
}

func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }

func TestScreenPlanFileErrors(t *testing.T) {
	a, b := modeltest.MergePair()
	// A plan for other databases.
	otherA, otherB := modeltest.MergePair()
	otherA.Meta.DatabaseName = "Other"
	other := plan.New(plan.Input{Hash: [32]byte{9}, Name: "x.kdbx"}, plan.Input{Hash: [32]byte{8}, Name: "y.kdbx"}, diff.Changes(otherA, otherB))
	var stale strings.Builder
	if err := plan.Write(&stale, other, plan.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		name   string
		button string
		path   func(h *harness) string
		want   string
		// route is the whole route across the goroutine of the plan action:
		// State.GoErr, called by loadPlan or savePlan on the UI thread, starts it there.
		route string
	}{
		{"load a file that is no plan", "Load plan…", func(*harness) string { return write("junk", "hello\n") },
			`The plan was not loaded: the plan, line 1: the first line must be "passbank-plan 1"`, "screenState.loadPlan->StateOf[...].GoErr->StateOf[...].GoErr.func1->screenState.loadPlan.func2->ReadFile->Read.PlanSyntax"},
		{"load a plan of other databases", "Load plan…", func(*harness) string { return write("stale", stale.String()) },
			"The plan was not loaded: the plan was not made for these databases: input A has changed since", "screenState.loadPlan->StateOf[...].GoErr->StateOf[...].GoErr.func1->screenState.loadPlan.func2->ReadFile->Read.PlanStale"},
		{"load a missing file", "Load plan…", func(*harness) string { return filepath.Join(dir, "missing") },
			"The plan was not loaded: missing cannot be read:", "screenState.loadPlan->StateOf[...].GoErr->StateOf[...].GoErr.func1->screenState.loadPlan.func2->ReadFile.PlanFileReadFailed"},
		{"save over database A", "Save plan…", func(h *harness) string {
			return write("a-copy.kdbx", "x") // replaced below by the path of A
		}, "", "screenState.savePlan->StateOf[...].GoErr->StateOf[...].GoErr.func1->screenState.savePlan.func3->CheckPath.PlanOutputIsInput"},
		{"save in a missing folder", "Save plan…", func(*harness) string { return filepath.Join(dir, "no", "plan.txt") },
			"The plan was not saved: plan.txt cannot be written:", "screenState.savePlan->StateOf[...].GoErr->StateOf[...].GoErr.func1->screenState.savePlan.func3->WriteFile.PlanFileWriteFailed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, kit.LightTheme(), sessionOf(a.Clone(), b.Clone()))
			path := tt.path(h)
			if tt.name == "save over database A" {
				if err := os.WriteFile(filepath.Join(dir, "db-a.kdbx"), []byte("database"), 0o600); err != nil {
					t.Fatal(err)
				}
				h.sess.A.Path = filepath.Join(dir, "db-a.kdbx")
				path = h.sess.A.Path
				tt.want = "The plan was not saved: the plan file must not be one of the databases"
			}
			before := slices.Clone(h.sess.Plan.Choices)
			h.planDialog(tt.button, path)
			if got := h.noteText(); !strings.HasPrefix(got, tt.want) {
				t.Errorf("note = %q, want it to start with %q", got, tt.want)
			}
			if n := h.state().note; n == nil {
				t.Error("no note")
			} else {
				assertRoute(t, n.err, tt.route)
			}
			if !slices.Equal(h.sess.Plan.Choices, before) {
				t.Error("a failed plan action changed the choices")
			}
			if h.model().Undecided() != 9 {
				t.Errorf("undecided = %d after a failure", h.model().Undecided())
			}
			if tt.name == "save over database A" {
				if b, _ := os.ReadFile(path); string(b) != "database" {
					t.Errorf("the database was overwritten: %q", b)
				}
			}
		})
	}
}

func TestScreenCancelledPlanDialogChangesNothing(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	for _, button := range []string{"Load plan…", "Save plan…"} {
		h.ts.Tap(gluttertest.ByText(button))
		h.ts.PumpAndSettle()
		h.waitFor("the dialog", func() bool { return h.fd.PendingFileChoosers() == 1 })
		h.fd.CancelFileChooser()
		h.waitFor("the dialog to close", func() bool { return h.fd.PendingFileChoosers() == 0 && !h.state().busy })
		if h.noteText() != "" {
			t.Errorf("%s: a cancelled dialog left the note %q", button, h.noteText())
		}
	}
	if h.model().Undecided() != 9 {
		t.Errorf("undecided = %d", h.model().Undecided())
	}
}

func TestScreenPlanButtonsWaitForTheDialog(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	h.ts.Tap(gluttertest.ByText("Save plan…"))
	h.ts.PumpAndSettle()
	h.waitFor("the dialog", func() bool { return h.fd.PendingFileChoosers() == 1 })
	if h.button("Load plan…").Enabled() || h.button("Save plan…").Enabled() {
		t.Error("the plan buttons are enabled while a dialog is open")
	}
	h.fd.CancelFileChooser()
	h.waitFor("the dialog to close", func() bool { return !h.state().busy })
	if !h.button("Load plan…").Enabled() {
		t.Error("the plan buttons stay disabled")
	}
}

func TestScreenDroppedGroupWithKeptItems(t *testing.T) {
	a, b := modeltest.MergePair()
	h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
	if h.ts.Exists(gluttertest.ByText("Drop everything in this group")) {
		t.Fatal("a fix is offered although nothing conflicts")
	}
	// Drop the group Games (only B has it) and keep the entry in it.
	h.tapRow("Games (group)")
	h.key(services.LogicalKeyboardKeyKeyD)
	want := "The group “Games (group)” is dropped, but 1 item in it is kept. The new database cannot be written like this."
	if !h.ts.Exists(gluttertest.ByText(want)) {
		t.Fatalf("no warning %q", want)
	}
	if err := merge.Validate(h.sess.Plan); err == nil {
		t.Fatal("the plan with a conflict is valid")
	}
	h.ts.Tap(gluttertest.ByText("Drop everything in this group"))
	h.ts.PumpAndSettle()
	if h.ts.Exists(gluttertest.ByText("Drop everything in this group")) {
		t.Error("the warning stays after the fix")
	}
	if h.choice(2) != plan.Drop || !h.sess.Decided[2] {
		t.Errorf("the entry in the dropped group got %q", h.choice(2))
	}
	if err := merge.Validate(h.sess.Plan); err != nil {
		t.Errorf("the plan is still not valid after the fix: %v", err)
	}
	if h.choice(0) != plan.Keep {
		t.Errorf("the fix changed another row: %q", h.choice(0))
	}
}

func TestModelDropEverythingInNestedGroups(t *testing.T) {
	a, b := modeltest.MergePair()
	// B gets Games/Arcade (a group) with an entry in it, so dropping Games takes two steps.
	var games *model.Group
	for _, g := range b.Root.Groups {
		if g.Name == "Games" {
			games = g
		}
	}
	arcade := &model.Group{UUID: model.UUID{0: 0xab, 1: 0xdd, 15: 1}, Name: "Arcade", Times: games.Times}
	arcade.Entries = []*model.Entry{games.Entries[0].Clone()}
	arcade.Entries[0].UUID = model.UUID{0: 0xab, 1: 0xdd, 15: 2}
	arcade.Entries[0].Fields[0].Value = secret.New([]byte("Pinball"))
	games.Groups = append(games.Groups, arcade)
	m := NewModel(planOf(a, b), nil)
	var gamesIdx int
	for i, c := range m.Plan().Changes {
		if c.Item == diff.ItemGroup && c.Title == "Games" {
			gamesIdx = i
		}
	}
	m.Set(gamesIdx, plan.Drop)
	conflicts := m.GroupConflicts()
	if len(conflicts) != 1 || conflicts[0].Group != gamesIdx || len(conflicts[0].Items) < 2 {
		t.Fatalf("conflicts = %+v", conflicts)
	}
	n := m.DropEverythingIn(gamesIdx)
	if n < 3 {
		t.Errorf("dropped %d changes, want the entry, the group Arcade and its entry", n)
	}
	if len(m.GroupConflicts()) != 0 {
		t.Errorf("conflicts remain: %+v", m.GroupConflicts())
	}
	if err := merge.Validate(m.Plan()); err != nil {
		t.Errorf("the plan is not valid: %v", err)
	}
}

// TestDetailsGolden shows the details of one change of every kind, in the light
// theme, with fixture data only. The first one is the proof image of step 8.4: an
// entry that differs in its password (masked) and its URL.
func TestDetailsGolden(t *testing.T) {
	tests := []struct {
		golden string
		row    string
	}{
		{"details-differs", "Banking / My bank"},
		{"details-differs-title", "Shopping / Store"},
		{"details-only-a", "Email / Work mail"},
		{"details-only-b", "Games / Old forum"},
		{"details-only-b-group", "Games (group)"},
		{"details-deleted-in-a", "Social / Old account"},
		{"details-deleted-in-b", "Social / Stale login"},
		{"details-group-differs", "Shopping (group)"},
		{"details-settings", "Database settings"},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			a, b := pairWithAttachmentsFor(t)
			h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
			h.tapRow(tt.row)
			h.ts.MatchesGolden(tt.golden)
			if tt.golden == "details-differs" {
				testutil.Proof(t, h.ts, "8.4-review-details")
			}
		})
	}
}

// pairWithAttachmentsFor is the merge fixtures with the attachments of
// pairWithAttachments on "My bank".
func pairWithAttachmentsFor(t *testing.T) (a, b *model.Database) {
	t.Helper()
	a, b, _ = pairWithAttachments(t)
	return a, b
}

// A very long group path and title are cut with an ellipsis in the row and in the
// details, and nothing overflows (the tester fails on an overflow), at both text
// sizes.
func TestScreenLongNames(t *testing.T) {
	long := "A very long name that goes on and on and on and never seems to stop"
	for _, sc := range []struct {
		name   string
		factor float64
	}{{"100", 1}, {"200", 2}} {
		t.Run(sc.name, func(t *testing.T) {
			a, b := modeltest.MergePair()
			for _, db := range []*model.Database{a, b} {
				bank := db.Root.Groups[1]
				bank.Name = long + " (group)"
				bank.Entries[0].Fields[0].Value = secret.New([]byte(long + " (entry)"))
				for _, f := range bank.Entries[0].Fields[3:4] {
					f.Value = secret.New([]byte("https://example.com/" + strings.Repeat("path/", 30)))
				}
			}
			h := newHarness(t, kit.LightTheme(), sessionOf(a, b))
			h.ts.SetPlatformConfiguration(func(c *ui.PlatformConfiguration) { c.TextScaleFactor = sc.factor })
			h.ts.PumpAndSettle()
			h.tapRow(Title(h.model().Change(3)))
			h.ts.MatchesGolden("review-long-names-" + sc.name)
		})
	}
}
