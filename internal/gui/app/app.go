package app

import (
	"context"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/animation"
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/rendering"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/kit/fonts"
	"github.com/rm4n0s/passbank/internal/gui/open"
	"github.com/rm4n0s/passbank/internal/gui/session"
)

// The identity of the application.
const (
	// AppID is the Wayland app_id and the X11 WM_CLASS; it matches the .desktop
	// file of the application.
	AppID = "dev.rm4n0s.Passbank"
	// Title is the title of the window.
	Title = "Passbank"
)

// Run opens the window and runs the application until the window is closed or
// ctx ends. It must be called on the main goroutine.
func Run(ctx context.Context) error {
	err := widgets.RunAppContext(ctx, NewApp(), widgets.RunAppOpts{
		AppID: AppID,
		Window: widgets.WindowConfig{
			Title:   Title,
			Size:    ui.Size{Width: 1100, Height: 720},
			MinSize: ui.Size{Width: 640, Height: 480},
		},
		Fonts: fonts.Sources(),
	})
	if err != nil {
		return errors.NewErr("AppRunFailed", err)
	}
	return nil
}

// App is the root widget: a WidgetsApp, without Material, whose home is the screen
// the user is on. It owns the session and wipes it when it goes.
type App struct {
	widgets.StatefulWidget
}

// NewApp returns the root widget of the application.
func NewApp(opts ...widgets.WidgetOpts) *App {
	return &App{StatefulWidget: widgets.NewStatefulWidgetBase(opts...)}
}

// CreateState makes the state of the application.
func (a *App) CreateState() widgets.State { return &appState{} }

// screen is the screen the user is on.
type screen int

const (
	screenOpen screen = iota
	screenReview
	screenWrite
)

type appState struct {
	widgets.StateOf[*App]
	sess   *session.Session
	screen screen
}

func (s *appState) InitState() { s.sess = session.New() }

// Dispose drops every unlocked database and its secrets (SPEC §13: closing the window).
func (s *appState) Dispose() { s.sess.Wipe() }

// goTo shows another screen.
func (s *appState) goTo(to screen) { s.SetState(func() { s.screen = to }) }

// Build builds the WidgetsApp with one route, whose page is the current screen.
func (s *appState) Build(bc widgets.BuildContext) widgets.Widget {
	return widgets.NewWidgetsApp(kit.LightTheme().Background, widgets.WidgetsAppOpts{
		Title: Title,
		// The window is a proof image: it has no debug ribbon.
		DebugShowCheckedModeBanner: foundation.Some(false),
		TextStyle:                  kit.LightTheme().Body(),
		OnGenerateRoute: func(settings widgets.RouteSettings) widgets.Route {
			return widgets.NewPageRouteBuilder(
				func(bc widgets.BuildContext, _, _ animation.Animation[float64]) widgets.Widget {
					return s.page(bc)
				},
				widgets.PageRouteBuilderOpts{PageRouteOpts: widgets.PageRouteOpts{RouteOpts: widgets.RouteOpts{Settings: settings}}})
		},
	})
}

// page is the current screen in the theme of the desktop.
func (s *appState) page(bc widgets.BuildContext) widgets.Widget {
	theme := kit.LightTheme()
	if widgets.MediaQueryPlatformBrightnessOf(bc) == ui.BrightnessDark {
		theme = kit.DarkTheme()
	}
	var body widgets.Widget
	switch s.screen {
	case screenReview:
		body = placeholder("Review", func() { s.goTo(screenWrite) }, func() { s.sess.Wipe(); s.goTo(screenOpen) })
	case screenWrite:
		body = placeholder("Write", nil, func() { s.goTo(screenReview) })
	default:
		body = open.NewScreen(s.sess, func() { s.goTo(screenReview) })
	}
	return kit.NewRoot(theme, body)
}

// placeholder stands for a screen that a later step builds.
func placeholder(name string, next, back func()) widgets.Widget {
	return widgets.NewCenter(widgets.NewColumn([]widgets.Widget{
		widgets.NewText(name),
		kit.NewButton("Back", back),
		kit.NewButton("Next", next),
	}, widgets.ColumnOpts{MainAxisSize: rendering.MainAxisSizeMin}))
}
