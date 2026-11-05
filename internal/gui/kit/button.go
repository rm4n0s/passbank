package kit

import (
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/gestures"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/services"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"
)

// ButtonKind is how prominent a button is.
type ButtonKind int

const (
	// Secondary is an ordinary button.
	Secondary ButtonKind = iota
	// Primary is the main action of a screen.
	Primary
	// Danger is an action that loses something.
	Danger
)

// ButtonOpts are the optional parameters of NewButton.
type ButtonOpts struct {
	widgets.WidgetOpts
	// Kind is how prominent the button is; the default is Secondary.
	Kind ButtonKind
	// SemanticsLabel names the button for a screen reader when its label is not
	// enough (an icon, a symbol); the default is the label.
	SemanticsLabel string
	// Autofocus asks for the focus when the button is built.
	Autofocus bool
}

// Button is a labelled button. It is disabled when it has no action. It takes the
// keyboard focus, shows it with a ring, and acts on Enter and Space.
type Button struct {
	widgets.StatefulWidget
	label     string
	onPressed func()
	opts      ButtonOpts
}

// NewButton returns a button; a nil onPressed disables it.
func NewButton(label string, onPressed func(), opts ...ButtonOpts) *Button {
	var o ButtonOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &Button{StatefulWidget: widgets.NewStatefulWidgetBase(o.WidgetOpts), label: label, onPressed: onPressed, opts: o}
}

// Label is the text of the button.
func (b *Button) Label() string { return b.label }

// Enabled says whether the button has an action.
func (b *Button) Enabled() bool { return b.onPressed != nil }

// CreateState makes the state of the button.
func (b *Button) CreateState() widgets.State { return &buttonState{} }

type buttonState struct {
	widgets.StateOf[*Button]
	hovered, focused, pressed bool
}

func (s *buttonState) Build(bc widgets.BuildContext) widgets.Widget {
	b := s.Widget()
	th := ThemeOf(bc)
	enabled := b.onPressed != nil

	bg, fg, border := th.Surface, th.Text, th.Border
	switch b.opts.Kind {
	case Primary:
		bg, fg, border = th.Primary, th.OnPrimary, th.Primary
	case Danger:
		bg, fg, border = th.DangerSurface, th.Danger, th.Danger
	}
	switch {
	case !enabled:
		bg, fg, border = th.Raised, th.Disabled, th.Border
	case s.pressed || s.hovered:
		if b.opts.Kind == Secondary {
			bg = th.Raised
		} else {
			border = th.Focus
		}
	}
	ring, width := border, 1.0
	if s.focused && enabled {
		ring, width = th.Focus, 2.0
	}

	activate := func() {
		if b.onPressed != nil {
			b.onPressed()
		}
	}
	label := widgets.NewText(b.label, widgets.TextOpts{
		Style: TextStyle(SizeBody, ui.FontWeightW700, fg), MaxLines: foundation.Some(1), Overflow: painting.TextOverflowEllipsis,
	})
	body := widgets.NewContainer(widgets.NewCenter(label, widgets.CenterOpts{WidthFactor: foundation.Some(1.0), HeightFactor: foundation.Some(1.0)}), widgets.ContainerOpts{
		Height:  foundation.Some(ControlHeight),
		Padding: painting.NewEdgeInsetsSymmetric(0, SpaceL),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color:        foundation.Some(bg),
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(ring), Width: foundation.Some(width)}),
			BorderRadius: painting.NewBorderRadiusCircular(Radius),
		}),
	})
	var tap widgets.Widget = widgets.NewGestureDetector(body, widgets.GestureDetectorOpts{
		OnTap:       func() { activate() },
		OnTapDown:   func(gestures.TapDownDetails) { s.SetState(func() { s.pressed = enabled }) },
		OnTapUp:     func(gestures.TapUpDetails) { s.SetState(func() { s.pressed = false }) },
		OnTapCancel: func() { s.SetState(func() { s.pressed = false }) },
	})
	cursor := services.MouseCursor(services.SystemMouseCursors.Basic)
	if enabled {
		cursor = services.SystemMouseCursors.Click
	}
	tap = widgets.NewMouseRegion(tap, widgets.MouseRegionOpts{
		Cursor:  cursor,
		OnEnter: func(*gestures.PointerEnterEvent) { s.SetState(func() { s.hovered = true }) },
		OnExit:  func(*gestures.PointerExitEvent) { s.SetState(func() { s.hovered = false; s.pressed = false }) },
	})
	focus := widgets.NewFocus(tap, widgets.FocusOpts{
		Autofocus:       b.opts.Autofocus,
		CanRequestFocus: foundation.Some(enabled),
		OnFocusChange:   func(has bool) { s.SetState(func() { s.focused = has }) },
		OnKeyEvent: activateOnEnterOrSpace(func() {
			if enabled {
				activate()
			}
		}),
		ExcludeSemantics: true,
	})
	label2 := b.opts.SemanticsLabel
	if label2 == "" {
		label2 = b.label
	}
	return widgets.NewSemantics(focus, widgets.SemanticsOpts{
		Container: true, Button: foundation.Some(true), Enabled: foundation.Some(enabled),
		Focusable: foundation.Some(enabled), Focused: foundation.Some(s.focused),
		Label: foundation.Some(label2), OnTap: b.onPressed,
	})
}
