package kit_test

import (
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/services"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/gui/kit"
)

func TestChip(t *testing.T) {
	tests := []struct {
		name string
		act  func(ts *gluttertest.Tester)
		want int
	}{
		{"tap", func(ts *gluttertest.Tester) { ts.Tap(gluttertest.ByText("Differs 2")) }, 1},
		{"enter on the focused chip", func(ts *gluttertest.Tester) {
			ts.SendKeyEvent(services.LogicalKeyboardKeyEnter)
		}, 1},
		{"space on the focused chip", func(ts *gluttertest.Tester) {
			ts.SendKeyEvent(services.LogicalKeyboardKeySpace)
		}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toggled := 0
			ts := pump(t, kit.LightTheme(), ui.Size{Width: 300, Height: 80},
				widgets.NewRow([]widgets.Widget{kit.NewChip("Differs 2", true, func() { toggled++ }, kit.ChipOpts{Autofocus: true}), kit.NewChip("Settings 1", false, nil)}))
			tt.act(ts)
			ts.PumpAndSettle()
			if toggled != tt.want {
				t.Errorf("toggled %d times, want %d", toggled, tt.want)
			}
			ts.Tap(gluttertest.ByText("Settings 1")) // a chip without an action does nothing
		})
	}
}

func TestChipGolden(t *testing.T) {
	for _, th := range []struct {
		name  string
		theme kit.Theme
	}{{"light", kit.LightTheme()}, {"dark", kit.DarkTheme()}} {
		t.Run(th.name, func(t *testing.T) {
			ts := pump(t, th.theme, ui.Size{Width: 360, Height: 60}, widgets.NewRow([]widgets.Widget{
				kit.NewChip("All 9", true, func() {}), kit.NewChip("Differs 2", false, func() {}), kit.NewChip("Undecided 7", false, func() {}),
			}))
			ts.MatchesGolden("chip-" + th.name)
		})
	}
}
