package kit

import (
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/rendering"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"
)

// Masked is what a protected value is shown as while it is hidden.
const Masked = "••••••••"

// FieldRow is one row of a FieldTable: a field with the value in A and in B.
type FieldRow struct {
	// Name is the name of the field ("Title", "Password", "Attachments").
	Name string
	// A and B are the values; a value that only one side has leaves the other empty
	// and Missing says which.
	A, B string
	// MissingA and MissingB say that the field does not exist on that side, as
	// opposed to being empty.
	MissingA, MissingB bool
	// Differs highlights the row.
	Differs bool
	// Protected masks the values until the user reveals them with the toggle of the
	// row.
	Protected bool
}

// FieldTable shows the fields of two versions of an item side by side: the name,
// the value in A and the value in B, with the rows that differ highlighted. The
// values of protected rows are masked, each row with its own toggle that reveals
// them; the toggles are reset when ResetKey changes (the screen sets it to what is
// selected, so a value never stays revealed on another item).
type FieldTable struct {
	widgets.StatefulWidget
	rows     []FieldRow
	headerA  string
	headerB  string
	resetKey any
}

// NewFieldTable returns the table. headerA and headerB name the columns; resetKey
// identifies what the rows belong to.
func NewFieldTable(rows []FieldRow, headerA, headerB string, resetKey any, opts ...widgets.WidgetOpts) *FieldTable {
	return &FieldTable{StatefulWidget: widgets.NewStatefulWidgetBase(opts...), rows: rows, headerA: headerA, headerB: headerB, resetKey: resetKey}
}

// Rows are the rows of the table.
func (t *FieldTable) Rows() []FieldRow { return t.rows }

// CreateState makes the state of the table.
func (t *FieldTable) CreateState() widgets.State { return &fieldTableState{revealed: map[int]bool{}} }

type fieldTableState struct {
	widgets.StateOf[*FieldTable]
	revealed map[int]bool
}

func (s *fieldTableState) DidUpdateWidget(old *FieldTable) {
	if s.Widget().resetKey != old.resetKey {
		s.revealed = map[int]bool{}
	}
}

func (s *fieldTableState) Build(bc widgets.BuildContext) widgets.Widget {
	t := s.Widget()
	th := ThemeOf(bc)
	cell := func(text string, style *painting.TextStyle) widgets.Widget {
		return widgets.NewText(text, widgets.TextOpts{Style: style})
	}
	pad := painting.NewEdgeInsetsSymmetric(SpaceS, SpaceM)
	header := widgets.NewPadding(pad, widgets.NewRow([]widgets.Widget{
		widgets.NewSizedBox(cell("", th.Caption()), widgets.SizedBoxOpts{Width: foundation.Some(130.0)}),
		widgets.NewExpanded(cell(t.headerA, th.Bold())),
		widgets.NewExpanded(cell(t.headerB, th.Bold())),
		widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: foundation.Some(64.0)}),
	}))
	rows := []widgets.Widget{header}
	for i, r := range t.rows {
		i, r := i, r
		show := func(value string, missing bool) string {
			switch {
			case missing:
				return "(none)"
			case r.Protected && !s.revealed[i]:
				if value == "" {
					return ""
				}
				return Masked
			}
			return value
		}
		valueStyle := th.Body()
		if r.Protected && s.revealed[i] {
			valueStyle = TextStyle(SizeBody, ui.FontWeightW400, th.Text)
		}
		var toggle widgets.Widget = widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: foundation.Some(64.0)})
		if r.Protected {
			label, sem := "Show", "Show the "+r.Name+" of A and B"
			if s.revealed[i] {
				label, sem = "Hide", "Hide the "+r.Name+" of A and B"
			}
			toggle = widgets.NewSizedBox(NewLinkButton(label, sem, func() { s.SetState(func() { s.revealed[i] = !s.revealed[i] }) }),
				widgets.SizedBoxOpts{Width: foundation.Some(64.0)})
		}
		bg := th.Surface
		if r.Differs {
			bg = th.Differs
		}
		rows = append(rows, widgets.NewSemantics(widgets.NewContainer(widgets.NewRow([]widgets.Widget{
			widgets.NewSizedBox(cell(r.Name, th.Bold()), widgets.SizedBoxOpts{Width: foundation.Some(130.0)}),
			widgets.NewExpanded(widgets.NewPadding(painting.NewEdgeInsetsFromLTRB(0, 0, SpaceM, 0), widgets.NewText(show(r.A, r.MissingA), widgets.TextOpts{Style: valueStyle}))),
			widgets.NewExpanded(widgets.NewPadding(painting.NewEdgeInsetsFromLTRB(0, 0, SpaceM, 0), widgets.NewText(show(r.B, r.MissingB), widgets.TextOpts{Style: valueStyle}))),
			toggle,
		}, widgets.RowOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStart}), widgets.ContainerOpts{
			Padding: pad,
			Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
				Color:  foundation.Some(bg),
				Border: painting.Border{Bottom: painting.NewBorderSide(painting.BorderSideOpts{Color: foundation.Some(th.Border)})},
			}),
		}), widgets.SemanticsOpts{Container: true, Label: foundation.Some(rowLabel(r)), ExcludeSemantics: false}))
	}
	return widgets.NewColumn(rows, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch})
}

// rowLabel is what a screen reader says for a row: never a protected value.
func rowLabel(r FieldRow) string {
	state := "same in A and B"
	if r.Differs {
		state = "differs between A and B"
	}
	return r.Name + ", " + state
}
