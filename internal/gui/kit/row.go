package kit

import (
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/gestures"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/services"
	"github.com/rm4n0s/glutter/widgets"
)

// Modifiers say which modifier keys were held when a row was tapped.
type Modifiers struct {
	// Shift extends the selection from the anchor to the row.
	Shift bool
	// Ctrl adds the row to the selection or removes it.
	Ctrl bool
}

// ListRow is a row of a list that can be selected: it paints a background when
// selected or hovered, takes taps with the modifier keys that were held, and shows
// the keyboard focus. Its child is the content of the row.
type ListRow struct {
	widgets.StatefulWidget
	child    widgets.Widget
	label    string
	selected bool
	onTap    func(Modifiers)
}

// NewListRow returns a row; label is what a screen reader says for it.
func NewListRow(child widgets.Widget, label string, selected bool, onTap func(Modifiers), opts ...widgets.WidgetOpts) *ListRow {
	return &ListRow{StatefulWidget: widgets.NewStatefulWidgetBase(opts...), child: child, label: label, selected: selected, onTap: onTap}
}

// Selected says whether the row is selected.
func (r *ListRow) Selected() bool { return r.selected }

// CreateState makes the state of the row.
func (r *ListRow) CreateState() widgets.State { return &listRowState{} }

type listRowState struct {
	widgets.StateOf[*ListRow]
	hovered bool
}

func (s *listRowState) Build(bc widgets.BuildContext) widgets.Widget {
	r := s.Widget()
	th := ThemeOf(bc)
	bg := th.Surface
	switch {
	case r.selected:
		bg = th.Raised
	case s.hovered:
		bg = th.Background
	}
	border := painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(bg), Width: foundation.Some(2.0)})
	if r.selected {
		border = painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(th.Primary), Width: foundation.Some(2.0)})
	}
	body := widgets.NewContainer(r.child, widgets.ContainerOpts{
		Padding: painting.NewEdgeInsetsSymmetric(SpaceS, SpaceM),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color: foundation.Some(bg), Border: border, BorderRadius: painting.NewBorderRadiusCircular(Radius),
		}),
	})
	tap := func() {
		if r.onTap == nil {
			return
		}
		kb := widgets.WidgetsBindingOf(s.Context()).Keyboard()
		r.onTap(Modifiers{Shift: kb.IsShiftPressed(), Ctrl: kb.IsControlPressed()})
	}
	var w widgets.Widget = widgets.NewGestureDetector(body, widgets.GestureDetectorOpts{OnTap: tap})
	w = widgets.NewMouseRegion(w, widgets.MouseRegionOpts{
		Cursor:  services.SystemMouseCursors.Click,
		OnEnter: func(*gestures.PointerEnterEvent) { s.SetState(func() { s.hovered = true }) },
		OnExit:  func(*gestures.PointerExitEvent) { s.SetState(func() { s.hovered = false }) },
	})
	return widgets.NewSemantics(w, widgets.SemanticsOpts{
		Container: true, Selected: foundation.Some(r.selected), Label: foundation.Some(r.label),
		OnTap: func() { tap() },
	})
}
