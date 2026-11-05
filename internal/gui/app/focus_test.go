package app

import (
	"slices"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/services"

	"github.com/rm4n0s/passbank/internal/gui/testutil"
)

// focusOrder presses Tab n times and returns the label of what has the focus after
// each press.
func focusOrder(ts *gluttertest.Tester, n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		ts.SendKeyEvent(services.LogicalKeyboardKeyTab)
		ts.PumpAndSettle()
		label := "(nothing)"
		if node := ts.Binding.FocusManager().PrimaryFocus(); node != nil {
			label = node.DebugLabel()
		}
		out = append(out, label)
	}
	return out
}

// The keyboard reaches every control of every screen, panel by panel and top to
// bottom, and goes round again after the last (nothing traps the focus).
func TestFocusOrder(t *testing.T) {
	open := []string{
		"Database A file", "Choose… Database A file", "Password of database A", "Show the Password of database A",
		"Key file of database A", "Choose… Key file of database A", "Unlock",
		"Database B file", "Choose… Database B file", "Password of database B", "Show the Password of database B",
		"Key file of database B", "Choose… Key file of database B", "Unlock",
		"Swap A and B", // Compare has no action yet, so it takes no focus
		"Database A file",
	}
	ts := testutil.New(t)
	ts.PumpWidget(NewApp())
	ts.PumpAndSettle()
	if got := focusOrder(ts, len(open)); !slices.Equal(got, open) {
		t.Errorf("Open: focus order\n got %q\nwant %q", got, open)
	}

	review := []string{
		"Load plan…", "Save plan…",
		"All 9", "Only in A 1", "Only in B 2", "Differs 2", "Deleted in A 1", "Deleted in B 1", "Group differs 1", "Settings 1", "Undecided 9",
		"Search group path and title",
		"Newer wins", "A wins", "B wins", "Keep everything",
		"review list",
		"Back", "Next: write the new database",
		"Load plan…",
	}
	ts, _, _ = pumpWith(t, mergeFixtures(t))
	if got := focusOrder(ts, len(review)); !slices.Equal(got, review) {
		t.Errorf("Review: focus order\n got %q\nwant %q", got, review)
	}

	write := []string{
		"New database", "Choose… New database",
		"Version: KDBX 4", "Version: 4.1", "Version: 4.0", "Version: 3.1", "Version: 3.0", "Version: 2.0",
		"Database settings from: From A", "Database settings from: From B",
		"Credentials: As A", "Credentials: As B", "Credentials: New",
		"Encryption: As A", "Encryption: As B", "Encryption: Defaults of the version",
		"Back", // Write has no path yet, so it takes no focus
		"New database",
	}
	ts.Tap(gluttertest.ByText("Next: write the new database"))
	ts.PumpAndSettle()
	if got := focusOrder(ts, len(write)); !slices.Equal(got, write) {
		t.Errorf("Write: focus order\n got %q\nwant %q", got, write)
	}
}

// The window is named Passbank, has the application id of its .desktop file, and
// cannot be made smaller than the screens need.
func TestWindowConfiguration(t *testing.T) {
	c := windowConfig()
	if c.Title != "Passbank" || AppID != "dev.rm4n0s.Passbank" {
		t.Errorf("title %q, app id %q", c.Title, AppID)
	}
	if c.MinSize.Width < 640 || c.MinSize.Height < 480 {
		t.Errorf("minimum size %v is smaller than the screens are tested at", c.MinSize)
	}
	if c.Size.Width < c.MinSize.Width || c.Size.Height < c.MinSize.Height {
		t.Errorf("size %v is smaller than the minimum %v", c.Size, c.MinSize)
	}
}
