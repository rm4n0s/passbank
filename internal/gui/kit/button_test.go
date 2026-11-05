package kit_test

import (
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/services"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/gui/kit"
)

func TestButtonTapAndKeys(t *testing.T) {
	presses := 0
	ts := pump(t, kit.LightTheme(), ui.Size{Width: 300, Height: 100}, kit.NewButton("Unlock", func() { presses++ }, kit.ButtonOpts{Kind: kit.Primary, Autofocus: true}))
	ts.Tap(gluttertest.ByText("Unlock"))
	ts.PumpAndSettle()
	if presses != 1 {
		t.Fatalf("a tap pressed %d times", presses)
	}
	// The button has the focus (Autofocus), so the keys act on it.
	ts.SendKeyEvent(services.LogicalKeyboardKeyEnter)
	ts.SendKeyEvent(services.LogicalKeyboardKeySpace)
	ts.PumpAndSettle()
	if presses != 3 {
		t.Errorf("Enter and Space pressed %d times in all, want 3", presses)
	}
}

func TestButtonDisabled(t *testing.T) {
	ts := pump(t, kit.LightTheme(), ui.Size{Width: 300, Height: 100}, kit.NewButton("Compare", nil))
	ts.Tap(gluttertest.ByText("Compare"))
	ts.SendKeyEvent(services.LogicalKeyboardKeyTab)
	ts.SendKeyEvent(services.LogicalKeyboardKeyEnter)
	ts.PumpAndSettle()
	b := gluttertest.WidgetAs[*kit.Button](ts, gluttertest.ByType[*kit.Button]())
	if b.Enabled() {
		t.Error("a button without an action is enabled")
	}
}

func TestButtonGolden(t *testing.T) {
	for _, th := range themes {
		t.Run(th.name, func(t *testing.T) {
			ts := pump(t, th.theme, ui.Size{Width: 520, Height: 80}, widgets.NewRow([]widgets.Widget{
				kit.NewButton("Primary", func() {}, kit.ButtonOpts{Kind: kit.Primary}),
				widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: some(kit.SpaceM)}),
				kit.NewButton("Secondary", func() {}),
				widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: some(kit.SpaceM)}),
				kit.NewButton("Drop", func() {}, kit.ButtonOpts{Kind: kit.Danger}),
				widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: some(kit.SpaceM)}),
				kit.NewButton("Disabled", nil),
			}))
			ts.MatchesGolden("button-" + th.name)
		})
	}
}
