package kit

import (
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/gestures"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"
)

// Segment is one choice of a Segmented control.
type Segment struct {
	// Label is the text of the segment.
	Label string
	// Value identifies the segment in OnChanged and Selected.
	Value string
	// Disabled makes the segment impossible to choose.
	Disabled bool
}

// Segmented is a row of segments of which one is chosen, like radio buttons that sit
// side by side. Each segment takes the keyboard focus and is chosen with Enter or
// Space.
type Segmented struct {
	widgets.StatelessWidget
	label     string
	segments  []Segment
	selected  string
	onChanged func(value string)
}

// NewSegmented returns the control named label: the segments, the value of the one
// that is chosen (none when it matches no segment) and the function that hears the
// user's choice.
func NewSegmented(label string, segments []Segment, selected string, onChanged func(value string), opts ...widgets.WidgetOpts) *Segmented {
	return &Segmented{StatelessWidget: widgets.NewStatelessWidgetBase(opts...), label: label, segments: segments, selected: selected, onChanged: onChanged}
}

// Label is the name of the control.
func (s *Segmented) Label() string { return s.label }

// Segments are the choices of the control.
func (s *Segmented) Segments() []Segment { return s.segments }

// Selected is the value of the chosen segment.
func (s *Segmented) Selected() string { return s.selected }

// Build builds the row of segments.
func (s *Segmented) Build(bc widgets.BuildContext) widgets.Widget {
	// The segments wrap to a second line when they do not fit, as they do at a large
	// text size.
	items := make([]widgets.Widget, 0, len(s.segments))
	for _, seg := range s.segments {
		items = append(items, newSegmentView(s.label, seg, seg.Value == s.selected, s.onChanged))
	}
	return widgets.NewSemantics(widgets.NewWrap(items, widgets.WrapOpts{Spacing: SpaceS, RunSpacing: SpaceS}),
		widgets.SemanticsOpts{Container: true, ExplicitChildNodes: true, Label: foundation.Some(s.label)})
}

type segmentView struct {
	widgets.StatefulWidget
	group    string
	segment  Segment
	selected bool
	choose   func(string)
}

func newSegmentView(group string, seg Segment, selected bool, choose func(string)) *segmentView {
	return &segmentView{StatefulWidget: widgets.NewStatefulWidgetBase(), group: group, segment: seg, selected: selected, choose: choose}
}

func (v *segmentView) CreateState() widgets.State { return &segmentState{} }

type segmentState struct {
	widgets.StateOf[*segmentView]
	hovered, focused bool
}

func (s *segmentState) Build(bc widgets.BuildContext) widgets.Widget {
	v := s.Widget()
	th := ThemeOf(bc)
	enabled := !v.segment.Disabled && v.choose != nil
	bg, fg, border := th.Surface, th.Text, th.Border
	switch {
	case !enabled:
		bg, fg = th.Raised, th.Disabled
	case v.selected:
		bg, fg, border = th.Primary, th.OnPrimary, th.Primary
	case s.hovered:
		bg = th.Raised
	}
	width := 1.0
	if s.focused && enabled {
		border, width = th.Focus, 2.0
	}
	pick := func() {
		if enabled {
			v.choose(v.segment.Value)
		}
	}
	body := widgets.NewContainer(widgets.NewText(v.segment.Label, widgets.TextOpts{
		Style: TextStyle(SizeBody, ui.FontWeightW700, fg), MaxLines: foundation.Some(1),
	}), widgets.ContainerOpts{
		Height:  foundation.Some(Scaled(bc, ControlHeight)),
		Padding: painting.NewEdgeInsetsSymmetric(8, SpaceL),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color:        foundation.Some(bg),
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(border), Width: foundation.Some(width)}),
			BorderRadius: painting.NewBorderRadiusCircular(Radius),
		}),
	})
	var w widgets.Widget = widgets.NewGestureDetector(widgets.NewExcludeSemantics(body), widgets.GestureDetectorOpts{OnTap: pick, ExcludeFromSemantics: true})
	w = widgets.NewMouseRegion(w, widgets.MouseRegionOpts{
		OnEnter: func(*gestures.PointerEnterEvent) { s.SetState(func() { s.hovered = true }) },
		OnExit:  func(*gestures.PointerExitEvent) { s.SetState(func() { s.hovered = false }) },
	})
	w = widgets.NewFocus(w, widgets.FocusOpts{
		DebugLabel:       v.group + ": " + v.segment.Label,
		CanRequestFocus:  foundation.Some(enabled),
		OnFocusChange:    func(has bool) { s.SetState(func() { s.focused = has }) },
		OnKeyEvent:       activateOnEnterOrSpace(pick),
		ExcludeSemantics: true,
	})
	return widgets.NewSemantics(w, widgets.SemanticsOpts{
		Container: true, Button: foundation.Some(true), Enabled: foundation.Some(enabled), Selected: foundation.Some(v.selected),
		InMutuallyExclusiveGroup: foundation.Some(true), Focusable: foundation.Some(enabled), Focused: foundation.Some(s.focused),
		Label: foundation.Some(v.segment.Label), OnTap: pick,
	})
}
