package kit

import (
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/gui/kit/fonts"
)

// Theme holds the colors of the interface. The two themes, Light and Dark, are
// the same set of colors; the widgets read them with ThemeOf.
type Theme struct {
	// Dark says that the colors are the dark ones.
	Dark bool
	// Background is the color of the window.
	Background ui.Color
	// Surface is the color of panels and fields; Raised is a step above it (a
	// selected row, a hovered button).
	Surface, Raised ui.Color
	// Text is the main text color; Muted is for secondary text; Disabled for text
	// of things that cannot be used.
	Text, Muted, Disabled ui.Color
	// Border is the color of outlines; Focus the color of the focus ring.
	Border, Focus ui.Color
	// Primary is the accent color of the main button and the selection; OnPrimary
	// is the text on it.
	Primary, OnPrimary ui.Color
	// Danger and DangerSurface mark errors; Warning and WarningSurface warnings;
	// Success and SuccessSurface what went well.
	Danger, DangerSurface   ui.Color
	Warning, WarningSurface ui.Color
	Success, SuccessSurface ui.Color
	// Differs is the background of a field that differs between A and B.
	Differs ui.Color
}

// LightTheme returns the light colors.
func LightTheme() Theme {
	return Theme{
		Background: ui.NewColor(0xFFF4F5F7), Surface: ui.NewColor(0xFFFFFFFF), Raised: ui.NewColor(0xFFE8EAF0),
		Text: ui.NewColor(0xFF1B1B1F), Muted: ui.NewColor(0xFF5B5F6B), Disabled: ui.NewColor(0xFF9A9EAA),
		Border: ui.NewColor(0xFFC4C7D0), Focus: ui.NewColor(0xFF1F5FBF),
		Primary: ui.NewColor(0xFF1F5FBF), OnPrimary: ui.NewColor(0xFFFFFFFF),
		Danger: ui.NewColor(0xFFB3261E), DangerSurface: ui.NewColor(0xFFFBE4E2),
		Warning: ui.NewColor(0xFF8A5A00), WarningSurface: ui.NewColor(0xFFFFF0CC),
		Success: ui.NewColor(0xFF1B6E3C), SuccessSurface: ui.NewColor(0xFFDDF3E4),
		Differs: ui.NewColor(0xFFFFF0CC),
	}
}

// DarkTheme returns the dark colors.
func DarkTheme() Theme {
	return Theme{
		Dark:       true,
		Background: ui.NewColor(0xFF15161A), Surface: ui.NewColor(0xFF1F2026), Raised: ui.NewColor(0xFF2B2D36),
		Text: ui.NewColor(0xFFE6E7EC), Muted: ui.NewColor(0xFFA3A7B3), Disabled: ui.NewColor(0xFF6A6E7A),
		Border: ui.NewColor(0xFF3D4050), Focus: ui.NewColor(0xFF8AB4FF),
		Primary: ui.NewColor(0xFF8AB4FF), OnPrimary: ui.NewColor(0xFF0B1B3A),
		Danger: ui.NewColor(0xFFFFB4AB), DangerSurface: ui.NewColor(0xFF4A1B17),
		Warning: ui.NewColor(0xFFFFD27A), WarningSurface: ui.NewColor(0xFF4A3708),
		Success: ui.NewColor(0xFF8FDCA8), SuccessSurface: ui.NewColor(0xFF14361F),
		Differs: ui.NewColor(0xFF4A3708),
	}
}

// The spacing of the interface, in logical pixels.
const (
	// SpaceS is a small gap between things that belong together.
	SpaceS = 4.0
	// SpaceM is the usual gap.
	SpaceM = 8.0
	// SpaceL is the gap between groups.
	SpaceL = 16.0
	// SpaceXL is the margin of a screen.
	SpaceXL = 24.0
	// Radius is the corner radius of fields, buttons and panels.
	Radius = 6.0
	// ControlHeight is the height of buttons and fields.
	ControlHeight = 36.0
)

// The sizes of text.
const (
	// SizeBody is the size of ordinary text.
	SizeBody = 14.0
	// SizeSmall is the size of captions.
	SizeSmall = 12.0
	// SizeTitle is the size of a screen's title.
	SizeTitle = 22.0
)

// TextStyle returns a text style of the theme: the sans font, in the given size
// and weight, in the color.
func TextStyle(size float64, weight ui.FontWeight, color ui.Color) *painting.TextStyle {
	return painting.NewTextStyle(painting.TextStyleOpts{
		FontFamily: fonts.Sans,
		FontSize:   foundation.Some(size),
		Color:      foundation.Some(color),
		FontWeight: weight,
	})
}

// Body is the style of ordinary text in the theme.
func (t Theme) Body() *painting.TextStyle { return TextStyle(SizeBody, ui.FontWeightW400, t.Text) }

// Bold is the style of ordinary text in bold.
func (t Theme) Bold() *painting.TextStyle { return TextStyle(SizeBody, ui.FontWeightW700, t.Text) }

// Caption is the style of secondary text.
func (t Theme) Caption() *painting.TextStyle {
	return TextStyle(SizeSmall, ui.FontWeightW400, t.Muted)
}

// Title is the style of a screen's title.
func (t Theme) Title() *painting.TextStyle { return TextStyle(SizeTitle, ui.FontWeightW700, t.Text) }

// ThemeScope gives a Theme to the widgets below it.
type ThemeScope struct {
	widgets.InheritedWidget
	theme Theme
}

// NewThemeScope returns the widget that gives theme to child and everything below.
func NewThemeScope(theme Theme, child widgets.Widget, opts ...widgets.WidgetOpts) *ThemeScope {
	return &ThemeScope{InheritedWidget: widgets.NewInheritedWidgetBase(child, opts...), theme: theme}
}

// Theme is the theme the scope gives.
func (s *ThemeScope) Theme() Theme { return s.theme }

// UpdateShouldNotify is whether the theme changed.
func (s *ThemeScope) UpdateShouldNotify(old widgets.Widget) bool {
	return s.theme != old.(*ThemeScope).theme
}

// ThemeOf returns the theme of the closest ThemeScope above bc, or the light
// theme when there is none. The context depends on it.
func ThemeOf(bc widgets.BuildContext) Theme {
	if s, ok := widgets.DependOn[*ThemeScope](bc); ok {
		return s.theme
	}
	return LightTheme()
}

// NewRoot gives the theme to child, makes the theme's body text the default text
// style (so that a plain Text is readable in both themes) and paints the theme's
// background behind it. The application's root widget is one.
func NewRoot(theme Theme, child widgets.Widget) widgets.Widget {
	return NewThemeScope(theme, widgets.NewDefaultTextStyle(theme.Body(), widgets.NewColoredBox(theme.Background, child)))
}
