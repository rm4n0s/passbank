package kit_test

import (
	"regexp"
	"testing"

	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/testutil"
)

// themes are the two themes every widget is tested in.
var themes = []struct {
	name  string
	theme kit.Theme
}{{"light", kit.LightTheme()}, {"dark", kit.DarkTheme()}}

// host puts a widget in a window of the theme: left to right text, the theme's
// background, a margin.
func host(theme kit.Theme, child widgets.Widget) widgets.Widget {
	return widgets.NewDirectionality(ui.TextDirectionLTR,
		kit.NewRoot(theme, widgets.NewPadding(painting.NewEdgeInsetsAll(kit.SpaceL), child)))
}

// pump shows the widget in a window of the given size.
func pump(t *testing.T, theme kit.Theme, size ui.Size, child widgets.Widget) *gluttertest.Tester {
	t.Helper()
	ts := testutil.New(t)
	ts.SetViewSize(size)
	ts.PumpWidget(host(theme, child))
	ts.PumpAndSettle()
	return ts
}

func some[T any](v T) foundation.Opt[T] { return foundation.Some(v) }

func regexpOf(s string) *regexp.Regexp { return regexp.MustCompile(s) }
