package kit

import (
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/gestures"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"
)

// ChipHeight is the height of a chip.
const ChipHeight = 28.0

// ChipOpts are the optional parameters of NewChip.
type ChipOpts struct {
	widgets.WidgetOpts
	// Autofocus asks for the focus when the chip is built.
	Autofocus bool
}

// Chip is a small toggle that filters a list: a pill with a label that is filled
// when it is on. It takes the keyboard focus and is switched with Enter or Space.
type Chip struct {
	widgets.StatefulWidget
	label    string
	on       bool
	onToggle func()
	opts     ChipOpts
}

// NewChip returns a chip named label that is on or off; onToggle hears a click.
func NewChip(label string, on bool, onToggle func(), opts ...ChipOpts) *Chip {
	var o ChipOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &Chip{StatefulWidget: widgets.NewStatefulWidgetBase(o.WidgetOpts), label: label, on: on, onToggle: onToggle, opts: o}
}

// Label is the text of the chip.
func (c *Chip) Label() string { return c.label }

// On says whether the chip is switched on.
func (c *Chip) On() bool { return c.on }

// CreateState makes the state of the chip.
func (c *Chip) CreateState() widgets.State { return &chipState{} }

type chipState struct {
	widgets.StateOf[*Chip]
	hovered, focused bool
}

func (s *chipState) Build(bc widgets.BuildContext) widgets.Widget {
	c := s.Widget()
	th := ThemeOf(bc)
	bg, fg, border := th.Surface, th.Text, th.Border
	switch {
	case c.on:
		bg, fg, border = th.Primary, th.OnPrimary, th.Primary
	case s.hovered:
		bg = th.Raised
	}
	width := 1.0
	if s.focused {
		border, width = th.Focus, 2.0
	}
	toggle := func() {
		if c.onToggle != nil {
			c.onToggle()
		}
	}
	body := widgets.NewContainer(widgets.NewCenter(widgets.NewText(c.label, widgets.TextOpts{
		Style: TextStyle(SizeSmall, ui.FontWeightW700, fg), MaxLines: foundation.Some(1),
	}), widgets.CenterOpts{WidthFactor: foundation.Some(1.0), HeightFactor: foundation.Some(1.0)}), widgets.ContainerOpts{
		Height:  foundation.Some(Scaled(bc, ChipHeight)),
		Padding: painting.NewEdgeInsetsSymmetric(0, SpaceM+SpaceS),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color:        foundation.Some(bg),
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(border), Width: foundation.Some(width)}),
			BorderRadius: painting.NewBorderRadiusCircular(ChipHeight / 2),
		}),
	})
	var w widgets.Widget = widgets.NewGestureDetector(widgets.NewExcludeSemantics(body), widgets.GestureDetectorOpts{OnTap: toggle, ExcludeFromSemantics: true})
	w = widgets.NewMouseRegion(w, widgets.MouseRegionOpts{
		OnEnter: func(*gestures.PointerEnterEvent) { s.SetState(func() { s.hovered = true }) },
		OnExit:  func(*gestures.PointerExitEvent) { s.SetState(func() { s.hovered = false }) },
	})
	w = widgets.NewFocus(w, widgets.FocusOpts{
		DebugLabel:       c.label,
		Autofocus:        c.opts.Autofocus,
		CanRequestFocus:  foundation.Some(c.onToggle != nil),
		OnFocusChange:    func(has bool) { s.SetState(func() { s.focused = has }) },
		OnKeyEvent:       activateOnEnterOrSpace(toggle),
		ExcludeSemantics: true,
	})
	return widgets.NewSemantics(w, widgets.SemanticsOpts{
		Container: true, Button: foundation.Some(true), Enabled: foundation.Some(c.onToggle != nil), Toggled: foundation.Some(c.on),
		Focusable: foundation.Some(c.onToggle != nil), Focused: foundation.Some(s.focused),
		Label: foundation.Some(c.label), OnTap: toggle,
	})
}
