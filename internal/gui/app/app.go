package app

import (
	"context"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/animation"
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/gui/kit/fonts"
)

// The identity of the application.
const (
	// AppID is the Wayland app_id and the X11 WM_CLASS; it matches the .desktop
	// file of the application.
	AppID = "dev.rm4n0s.Passbank"
	// Title is the title of the window.
	Title = "Passbank"
)

// Colors of the first window; step 8.1 replaces them with the theme of the kit.
var (
	background   = ui.NewColor(0xFFFAFAFA)
	onBackground = ui.NewColor(0xFF1B1B1F)
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

// App is the root widget: a WidgetsApp, without Material, whose home is the
// screen the user is on.
type App struct {
	widgets.StatelessWidget
}

// NewApp returns the root widget of the application.
func NewApp(opts ...widgets.WidgetOpts) *App {
	return &App{StatelessWidget: widgets.NewStatelessWidgetBase(opts...)}
}

// Build builds the WidgetsApp with one route, whose page is the home.
func (a *App) Build(bc widgets.BuildContext) widgets.Widget {
	return widgets.NewWidgetsApp(background, widgets.WidgetsAppOpts{
		Title: Title,
		// The window is a proof image: it has no debug ribbon.
		DebugShowCheckedModeBanner: foundation.Some(false),
		TextStyle:                  textStyle(16, ui.FontWeightW400),
		OnGenerateRoute: func(settings widgets.RouteSettings) widgets.Route {
			return widgets.NewPageRouteBuilder(
				func(widgets.BuildContext, animation.Animation[float64], animation.Animation[float64]) widgets.Widget {
					return home()
				},
				widgets.PageRouteBuilderOpts{PageRouteOpts: widgets.PageRouteOpts{RouteOpts: widgets.RouteOpts{Settings: settings}}})
		},
	})
}

// home is the page shown until the Open screen exists (step 8.2): the name of
// the application, centered. WidgetsApp paints no background of its own (its
// color is only the primary color that the system sees), so the page paints it.
func home() widgets.Widget {
	return widgets.NewColoredBox(background,
		widgets.NewCenter(widgets.NewText(Title, widgets.TextOpts{Style: textStyle(40, ui.FontWeightW700)})))
}

// textStyle is the style of the interface text.
func textStyle(size float64, weight ui.FontWeight) *painting.TextStyle {
	return painting.NewTextStyle(painting.TextStyleOpts{
		FontFamily: fonts.Sans,
		FontSize:   foundation.Some(size),
		Color:      foundation.Some(onBackground),
		FontWeight: weight,
	})
}
