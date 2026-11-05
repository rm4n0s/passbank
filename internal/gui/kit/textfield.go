package kit

import (
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/rendering"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"
)

// TextFieldOpts are the optional parameters of NewTextField.
type TextFieldOpts struct {
	widgets.WidgetOpts
	// Controller holds the text; without one the field makes its own. The caller
	// owns a controller that it gives.
	Controller *widgets.TextEditingController
	// FocusNode is the focus of the field; without one the field makes its own.
	FocusNode *widgets.FocusNode
	// Hint is shown, dimmed, while the field is empty.
	Hint string
	// Obscure hides the text, for a password.
	Obscure bool
	// ReadOnly makes the text not editable (it can still be selected and copied).
	ReadOnly bool
	// Autofocus asks for the focus when the field is built.
	Autofocus bool
	// Invalid draws the field as one whose content is wrong.
	Invalid bool
	// Trailing is a widget at the right end inside the field (a reveal toggle).
	Trailing widgets.Widget
	// OnChanged is called with the text each time it changes.
	OnChanged func(text string)
	// OnSubmitted is called with the text when the user presses Enter.
	OnSubmitted func(text string)
}

// TextField is a single-line text field on EditableText: a bordered box with a
// hint, a focus ring, an optional obscured text and an optional trailing widget.
// label names it for a screen reader.
type TextField struct {
	widgets.StatefulWidget
	label string
	opts  TextFieldOpts
}

// NewTextField returns a text field named label.
func NewTextField(label string, opts ...TextFieldOpts) *TextField {
	var o TextFieldOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &TextField{StatefulWidget: widgets.NewStatefulWidgetBase(o.WidgetOpts), label: label, opts: o}
}

// Label is the name of the field.
func (f *TextField) Label() string { return f.label }

// CreateState makes the state of the field.
func (f *TextField) CreateState() widgets.State { return &textFieldState{} }

type textFieldState struct {
	widgets.StateOf[*TextField]
	controller    *widgets.TextEditingController
	ownController bool
	focus         *widgets.FocusNode
	ownFocus      bool
	listener      foundation.ListenerHandle
	focused       bool
}

func (s *textFieldState) InitState() {
	o := s.Widget().opts
	s.controller, s.ownController = o.Controller, o.Controller == nil
	if s.controller == nil {
		s.controller = widgets.NewTextEditingController()
	}
	s.focus, s.ownFocus = o.FocusNode, o.FocusNode == nil
	if s.focus == nil {
		s.focus = widgets.NewFocusNode()
	}
	s.listener = s.controller.AddListener(func() { s.SetState(func() {}) })
}

// DidUpdateWidget follows a controller or a focus node that the caller replaced.
func (s *textFieldState) DidUpdateWidget(old *TextField) {
	o := s.Widget().opts
	if o.Controller != old.opts.Controller {
		s.controller.RemoveListener(s.listener)
		if s.ownController {
			s.controller.Dispose()
		}
		s.controller, s.ownController = o.Controller, o.Controller == nil
		if s.controller == nil {
			s.controller = widgets.NewTextEditingController()
		}
		s.listener = s.controller.AddListener(func() { s.SetState(func() {}) })
	}
	if o.FocusNode != old.opts.FocusNode {
		if s.ownFocus {
			s.focus.Dispose()
		}
		s.focus, s.ownFocus = o.FocusNode, o.FocusNode == nil
		if s.focus == nil {
			s.focus = widgets.NewFocusNode()
		}
	}
}

func (s *textFieldState) Dispose() {
	s.controller.RemoveListener(s.listener)
	if s.ownController {
		s.controller.Dispose()
	}
	if s.ownFocus {
		s.focus.Dispose()
	}
}

func (s *textFieldState) Build(bc widgets.BuildContext) widgets.Widget {
	f := s.Widget()
	th := ThemeOf(bc)
	o := f.opts
	text := widgets.NewEditableText(s.controller, s.focus, th.Body(), th.Focus, th.Border, widgets.EditableTextOpts{
		ObscureText: o.Obscure, ReadOnly: o.ReadOnly, Autofocus: o.Autofocus,
		OnChanged: o.OnChanged, OnSubmitted: o.OnSubmitted,
		SelectionColor: foundation.Some(ui.NewColor(0x661F5FBF)),
	})
	var field widgets.Widget = text
	if s.controller.Text() == "" && o.Hint != "" {
		field = widgets.NewStack([]widgets.Widget{
			widgets.NewIgnorePointer(widgets.NewText(o.Hint, widgets.TextOpts{Style: TextStyle(SizeBody, ui.FontWeightW400, th.Disabled), MaxLines: foundation.Some(1), Overflow: painting.TextOverflowEllipsis})),
			text,
		}, widgets.StackOpts{Fit: rendering.StackFitPassthrough})
	}
	row := []widgets.Widget{widgets.NewExpanded(widgets.NewCenter(field, widgets.CenterOpts{HeightFactor: foundation.Some(1.0)}))}
	if o.Trailing != nil {
		row = append(row, widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: foundation.Some(SpaceS)}), o.Trailing)
	}
	border, width := th.Border, 1.0
	switch {
	case o.Invalid:
		border, width = th.Danger, 2.0
	case s.focused:
		border, width = th.Focus, 2.0
	}
	box := widgets.NewContainer(widgets.NewRow(row, widgets.RowOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentCenter}), widgets.ContainerOpts{
		Height:  foundation.Some(ControlHeight),
		Padding: painting.NewEdgeInsetsSymmetric(0, SpaceM),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color:        foundation.Some(th.Surface),
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(border), Width: foundation.Some(width)}),
			BorderRadius: painting.NewBorderRadiusCircular(Radius),
		}),
	})
	return widgets.NewSemantics(
		widgets.NewFocus(box, widgets.FocusOpts{
			OnFocusChange:    func(has bool) { s.SetState(func() { s.focused = has }) },
			CanRequestFocus:  foundation.Some(false),
			SkipTraversal:    foundation.Some(true),
			ExcludeSemantics: true,
		}),
		widgets.SemanticsOpts{
			Container: true, TextField: foundation.Some(true), Label: foundation.Some(f.label),
			Obscured: foundation.Some(o.Obscure), ReadOnly: foundation.Some(o.ReadOnly),
			Focused: foundation.Some(s.focused),
		})
}
