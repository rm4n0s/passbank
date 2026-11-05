package review

import (
	"strconv"

	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/rendering"
	"github.com/rm4n0s/glutter/services"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/plan"
	"github.com/rm4n0s/passbank/internal/gui/kit"
)

// RowHeight is the height of one row of the list. Every row has the same height,
// so the list builds only the rows in view, however many changes there are.
const RowHeight = 44.0

// pageRows is how many rows Page Up and Page Down move.
const pageRows = 10

// List is the left side of the Review screen: the filter, the search, the bulk
// rules and the list of changes. It edits the model in place and calls onChanged
// after each edit, so that the screen around it rebuilds.
type List struct {
	widgets.StatefulWidget
	model     *Model
	onChanged func()
}

// NewList returns the list of the model's changes.
func NewList(model *Model, onChanged func(), opts ...widgets.WidgetOpts) *List {
	return &List{StatefulWidget: widgets.NewStatefulWidgetBase(opts...), model: model, onChanged: onChanged}
}

// CreateState makes the state of the list.
func (l *List) CreateState() widgets.State { return &listState{} }

type listState struct {
	widgets.StateOf[*List]
	scroll *widgets.ScrollControllerBase
	focus  *widgets.FocusNode
	search *widgets.TextEditingController
}

func (s *listState) InitState() {
	s.scroll = widgets.NewScrollController()
	s.focus = widgets.NewFocusNode(widgets.FocusNodeOpts{DebugLabel: "review list"})
	s.search = widgets.NewTextEditingController()
	s.search.SetText(s.Widget().model.Query())
}

func (s *listState) Dispose() {
	s.scroll.Dispose()
	s.focus.Dispose()
	s.search.Dispose()
}

// changed tells the screen that the model changed.
func (s *listState) changed() {
	if f := s.Widget().onChanged; f != nil {
		f()
	}
	s.SetState(func() {})
}

func (s *listState) Build(bc widgets.BuildContext) widgets.Widget {
	th := kit.ThemeOf(bc)
	m := s.Widget().model
	gap := func(h float64) widgets.Widget {
		return widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(h)})
	}
	chips := []widgets.Widget{
		kit.NewChip("All "+strconv.Itoa(m.Total()), !m.hasKind && !m.undecidedOnly, func() {
			m.ClearKind()
			m.SetUndecidedOnly(false)
			s.changed()
		}),
	}
	for _, k := range diff.Kinds {
		if m.KindCount(k) == 0 {
			continue
		}
		kind := k
		on := m.hasKind && m.kind == kind
		chips = append(chips, kit.NewChip(KindLabel(kind)+" "+strconv.Itoa(m.KindCount(kind)), on, func() {
			if on {
				m.ClearKind()
			} else {
				m.SetKind(kind)
			}
			s.changed()
		}))
	}
	chips = append(chips, kit.NewChip("Undecided "+strconv.Itoa(m.Undecided()), m.undecidedOnly, func() {
		m.SetUndecidedOnly(!m.undecidedOnly)
		s.changed()
	}))

	search := kit.NewTextField("Search group path and title", kit.TextFieldOpts{
		Controller: s.search, Hint: "Search group path and title",
		OnChanged: func(q string) {
			if q != m.Query() {
				m.SetQuery(q)
				s.changed()
			}
		},
	})

	rules := []widgets.Widget{}
	for _, r := range plan.Rules {
		rule := r
		rules = append(rules, kit.NewButton(RuleLabel(rule), func() {
			m.ApplyRule(rule)
			s.changed()
		}))
	}

	return widgets.NewColumn([]widgets.Widget{
		widgets.NewWrap(chips, widgets.WrapOpts{Spacing: kit.SpaceS, RunSpacing: kit.SpaceS}),
		gap(kit.SpaceM),
		search,
		gap(kit.SpaceM),
		widgets.NewText("Set the "+plural(m.Len(), "change")+" shown:", widgets.TextOpts{Style: th.Caption()}),
		gap(kit.SpaceS),
		widgets.NewWrap(rules, widgets.WrapOpts{Spacing: kit.SpaceS, RunSpacing: kit.SpaceS}),
		gap(kit.SpaceM),
		widgets.NewText(s.status(), widgets.TextOpts{Style: th.Caption()}),
		gap(kit.SpaceS),
		widgets.NewExpanded(s.rows(th)),
	}, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch})
}

// status is the line above the list: how many rows are shown and selected, and the
// keys.
func (s *listState) status() string {
	m := s.Widget().model
	text := strconv.Itoa(m.Len()) + " shown"
	if n := m.SelectedCount(); n > 0 {
		text += ", " + strconv.Itoa(n) + " selected"
	}
	return text + "   keys: A B K D N set the choice"
}

// rows is the list itself, with the keys.
func (s *listState) rows(th kit.Theme) widgets.Widget {
	m := s.Widget().model
	var body widgets.Widget
	if m.Len() == 0 {
		body = widgets.NewCenter(widgets.NewText(s.emptyText(), widgets.TextOpts{Style: th.Caption()}))
	} else {
		body = widgets.NewListViewBuilder(func(bc widgets.BuildContext, pos int) widgets.Widget {
			if pos < 0 || pos >= m.Len() {
				return nil
			}
			return s.row(th, pos)
		}, widgets.ListViewOpts{
			Controller: s.scroll, ItemCount: foundation.Some(m.Len()), ItemExtent: foundation.Some(RowHeight),
		})
	}
	box := widgets.NewContainer(body, widgets.ContainerOpts{
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color:        foundation.Some(th.Surface),
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(th.Border), Width: foundation.Some(1.0)}),
			BorderRadius: painting.NewBorderRadiusCircular(kit.Radius),
		}),
	})
	focus := widgets.NewFocus(box, widgets.FocusOpts{FocusNode: s.focus, OnKeyEvent: s.onKey, DebugLabel: "review list"})
	return widgets.NewSemantics(focus, widgets.SemanticsOpts{Container: true, ExplicitChildNodes: true, Label: foundation.Some("Changes")})
}

func (s *listState) emptyText() string {
	m := s.Widget().model
	switch {
	case m.Total() == 0:
		return "The two databases hold the same."
	case m.undecidedOnly && m.Undecided() == 0:
		return "Every change is decided."
	}
	return "No change matches."
}

// row is one row of the list.
func (s *listState) row(th kit.Theme, pos int) widgets.Widget {
	m := s.Widget().model
	c := m.Change(pos)
	choice := m.Choice(pos)
	decided := m.IsDecided(pos)

	choiceStyle := kit.TextStyle(kit.SizeBody, ui.FontWeightW700, th.Primary)
	if !decided {
		choiceStyle = kit.TextStyle(kit.SizeBody, ui.FontWeightW400, th.Muted)
	}
	one := func(style *painting.TextStyle, text string) widgets.Widget {
		return widgets.NewText(text, widgets.TextOpts{Style: style, MaxLines: foundation.Some(1), Overflow: painting.TextOverflowEllipsis})
	}
	cells := widgets.NewRow([]widgets.Widget{
		widgets.NewSizedBox(one(kit.TextStyle(kit.SizeSmall, ui.FontWeightW700, kindColor(th, c.Kind)), KindLabel(c.Kind)),
			widgets.SizedBoxOpts{Width: foundation.Some(104.0)}),
		widgets.NewExpanded(one(th.Body(), Title(c))),
		widgets.NewSizedBox(one(choiceStyle, ChoiceLabel(choice)), widgets.SizedBoxOpts{Width: foundation.Some(80.0)}),
	}, widgets.RowOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentCenter})

	state := "default"
	if decided {
		state = "decided"
	}
	label := KindLabel(c.Kind) + ", " + Title(c) + ", " + ChoiceLabel(choice) + ", " + state
	row := kit.NewListRow(cells, label, m.IsSelected(pos), func(mod kit.Modifiers) {
		m.Click(pos, mod.Shift, mod.Ctrl)
		s.focus.RequestFocus()
		s.changed()
	})
	return widgets.NewSizedBox(row, widgets.SizedBoxOpts{Height: foundation.Some(RowHeight)})
}

// kindColor is the color of a kind's name.
func kindColor(th kit.Theme, k diff.Kind) ui.Color {
	switch k {
	case diff.Differs, diff.GroupDiffers, diff.Settings:
		return th.Warning
	case diff.DeletedInA, diff.DeletedInB:
		return th.Danger
	}
	return th.Muted
}

// onKey handles the keys of the list: the arrows, Home, End, Page Up and Down,
// Ctrl+A, Escape, and the letters that set a choice.
func (s *listState) onKey(_ *widgets.FocusNode, ev services.KeyEvent) widgets.KeyEventResult {
	if _, down := ev.(*services.KeyDownEvent); !down {
		return widgets.KeyEventResultIgnored
	}
	m := s.Widget().model
	kb := widgets.WidgetsBindingOf(s.Context()).Keyboard()
	shift, ctrl := kb.IsShiftPressed(), kb.IsControlPressed()
	key := ev.LogicalKey()

	move := func(f func()) widgets.KeyEventResult {
		f()
		s.scrollToCursor()
		s.changed()
		return widgets.KeyEventResultHandled
	}
	act := func(a Action) widgets.KeyEventResult {
		m.Choose(a)
		s.scrollToCursor()
		s.changed()
		return widgets.KeyEventResultHandled
	}
	switch key {
	case services.LogicalKeyboardKeyArrowDown:
		return move(func() { m.Move(1, shift) })
	case services.LogicalKeyboardKeyArrowUp:
		return move(func() { m.Move(-1, shift) })
	case services.LogicalKeyboardKeyPageDown:
		return move(func() { m.Move(pageRows, shift) })
	case services.LogicalKeyboardKeyPageUp:
		return move(func() { m.Move(-pageRows, shift) })
	case services.LogicalKeyboardKeyHome:
		return move(func() { m.MoveTo(0, shift) })
	case services.LogicalKeyboardKeyEnd:
		return move(func() { m.MoveTo(-1, shift) })
	case services.LogicalKeyboardKeyEscape:
		return move(m.ClearSelection)
	}
	if ctrl {
		if key == services.LogicalKeyboardKeyKeyA {
			return move(m.SelectAll)
		}
		return widgets.KeyEventResultIgnored
	}
	switch key {
	case services.LogicalKeyboardKeyKeyA:
		return act(ActionA)
	case services.LogicalKeyboardKeyKeyB:
		return act(ActionB)
	case services.LogicalKeyboardKeyKeyK:
		return act(ActionKeep)
	case services.LogicalKeyboardKeyKeyD:
		return act(ActionDrop)
	case services.LogicalKeyboardKeyKeyN:
		return act(ActionNewer)
	}
	return widgets.KeyEventResultIgnored
}

// scrollToCursor scrolls the list so that the row with the cursor is in view.
func (s *listState) scrollToCursor() {
	m := s.Widget().model
	if m.Cursor() < 0 || !s.scroll.HasClients() {
		return
	}
	p := s.scroll.Position()
	if !p.HasViewportDimension() {
		return
	}
	top := float64(m.Cursor()) * RowHeight
	switch {
	case top < p.Pixels():
		s.scroll.JumpTo(top)
	case top+RowHeight > p.Pixels()+p.ViewportDimension():
		s.scroll.JumpTo(top + RowHeight - p.ViewportDimension())
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
