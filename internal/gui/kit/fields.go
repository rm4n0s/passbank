package kit

import (
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/rendering"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"
)

// PasswordField is a text field whose text is hidden, with a toggle at its end
// that reveals it. The screen reader hears the field as obscured while it is hidden.
type PasswordField struct {
	widgets.StatefulWidget
	label string
	opts  TextFieldOpts
}

// NewPasswordField returns a password field named label. The Obscure, ReadOnly and
// Trailing options are not used.
func NewPasswordField(label string, opts ...TextFieldOpts) *PasswordField {
	var o TextFieldOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &PasswordField{StatefulWidget: widgets.NewStatefulWidgetBase(o.WidgetOpts), label: label, opts: o}
}

// CreateState makes the state of the field.
func (p *PasswordField) CreateState() widgets.State { return &passwordFieldState{} }

type passwordFieldState struct {
	widgets.StateOf[*PasswordField]
	revealed bool
}

func (s *passwordFieldState) Build(bc widgets.BuildContext) widgets.Widget {
	p := s.Widget()
	o := p.opts
	o.Obscure = !s.revealed
	o.Trailing = NewLinkButton(map[bool]string{false: "Show", true: "Hide"}[s.revealed],
		map[bool]string{false: "Show the " + p.label, true: "Hide the " + p.label}[s.revealed],
		func() { s.SetState(func() { s.revealed = !s.revealed }) })
	return NewTextField(p.label, o)
}

// LinkButton is a small text-only button, used where a full button is too big.
type LinkButton struct {
	widgets.StatefulWidget
	label, semantics string
	onTap            func()
}

// NewLinkButton returns a text button; semantics is how a screen reader names it.
func NewLinkButton(label, semantics string, onTap func(), opts ...widgets.WidgetOpts) *LinkButton {
	return &LinkButton{StatefulWidget: widgets.NewStatefulWidgetBase(opts...), label: label, semantics: semantics, onTap: onTap}
}

// Label is the text of the button.
func (l *LinkButton) Label() string { return l.label }

// CreateState makes the state of the button.
func (l *LinkButton) CreateState() widgets.State { return &linkButtonState{} }

type linkButtonState struct {
	widgets.StateOf[*LinkButton]
	focused bool
}

func (s *linkButtonState) Build(bc widgets.BuildContext) widgets.Widget {
	l := s.Widget()
	th := ThemeOf(bc)
	enabled := l.onTap != nil
	color := th.Primary
	if !enabled {
		color = th.Disabled
	}
	decoration := painting.NewBoxDecoration(painting.BoxDecorationOpts{BorderRadius: painting.NewBorderRadiusCircular(Radius)})
	if s.focused {
		decoration = painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(th.Focus), Width: foundation.Some(2.0)}),
			BorderRadius: painting.NewBorderRadiusCircular(Radius),
		})
	}
	body := widgets.NewContainer(widgets.NewText(l.label, widgets.TextOpts{Style: TextStyle(SizeSmall, ui.FontWeightW700, color)}), widgets.ContainerOpts{
		Padding: painting.NewEdgeInsetsSymmetric(SpaceS, SpaceM), Decoration: decoration,
	})
	var w widgets.Widget = widgets.NewGestureDetector(body, widgets.GestureDetectorOpts{OnTap: func() {
		if l.onTap != nil {
			l.onTap()
		}
	}})
	w = widgets.NewFocus(w, widgets.FocusOpts{
		DebugLabel:       l.semantics,
		CanRequestFocus:  foundation.Some(enabled),
		OnFocusChange:    func(has bool) { s.SetState(func() { s.focused = has }) },
		OnKeyEvent:       activateOnEnterOrSpace(l.onTap),
		ExcludeSemantics: true,
	})
	return widgets.NewSemantics(w, widgets.SemanticsOpts{
		Container: true, Button: foundation.Some(true), Enabled: foundation.Some(enabled),
		Focusable: foundation.Some(enabled), Focused: foundation.Some(s.focused),
		Label: foundation.Some(l.semantics), OnTap: l.onTap,
	})
}

// FileFieldOpts are the optional parameters of NewFileField.
type FileFieldOpts struct {
	widgets.WidgetOpts
	// Controller holds the path; the caller owns it.
	Controller *widgets.TextEditingController
	// Hint is shown while the path is empty.
	Hint string
	// ChooseLabel is the text of the button; the default is "Choose…".
	ChooseLabel string
	// OnChanged is called with the path each time the user edits it.
	OnChanged func(path string)
}

// FileField is a path field with a button beside it that opens a file chooser. The
// widget does not open the chooser itself: onChoose does, and puts the answer in the
// controller.
type FileField struct {
	widgets.StatelessWidget
	label    string
	onChoose func()
	opts     FileFieldOpts
}

// NewFileField returns a path field named label whose button calls onChoose.
func NewFileField(label string, onChoose func(), opts ...FileFieldOpts) *FileField {
	var o FileFieldOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	return &FileField{StatelessWidget: widgets.NewStatelessWidgetBase(o.WidgetOpts), label: label, onChoose: onChoose, opts: o}
}

// Build builds the field and its button.
func (f *FileField) Build(bc widgets.BuildContext) widgets.Widget {
	choose := f.opts.ChooseLabel
	if choose == "" {
		choose = "Choose…"
	}
	return widgets.NewRow([]widgets.Widget{
		widgets.NewExpanded(NewTextField(f.label, TextFieldOpts{Controller: f.opts.Controller, Hint: f.opts.Hint, OnChanged: f.opts.OnChanged})),
		widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: foundation.Some(SpaceM)}),
		NewButton(choose, f.onChoose, ButtonOpts{SemanticsLabel: choose + " " + f.label}),
	}, widgets.RowOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentCenter})
}
