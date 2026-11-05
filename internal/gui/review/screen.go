package review

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/rendering"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/plan"
	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/session"
)

// ListWidth is the width of the list on the left of the screen.
const ListWidth = 560.0

// Screen is the Review screen (SPEC §13 screen 2). It compares the two databases of
// the session when the session has no plan yet, and lists the changes; the choices
// it records are in the session's plan, so going back to this screen from the Write
// screen keeps them.
type Screen struct {
	widgets.StatefulWidget
	sess *session.Session
	// onBack and onNext leave the screen: back to the Open screen, on to the Write
	// screen.
	onBack, onNext func()
}

// NewScreen returns the Review screen of the session.
func NewScreen(sess *session.Session, onBack, onNext func(), opts ...widgets.WidgetOpts) *Screen {
	return &Screen{StatefulWidget: widgets.NewStatefulWidgetBase(opts...), sess: sess, onBack: onBack, onNext: onNext}
}

// CreateState makes the state of the screen.
func (s *Screen) CreateState() widgets.State { return &screenState{} }

type screenState struct {
	widgets.StateOf[*Screen]
	// model is nil while the databases are being compared.
	model *Model
	// work is the comparison, which Dispose waits for before the session may be wiped.
	work sync.WaitGroup
}

func (s *screenState) InitState() {
	sess := s.Widget().sess
	if sess.Plan != nil {
		s.model = NewModel(sess.Plan, sess.Decided)
		return
	}
	if !sess.Ready() {
		return
	}
	a, b := sess.A, sess.B
	s.work.Add(1)
	s.Go(func(ctx context.Context) {
		defer s.work.Done()
		changes := diff.Changes(a.DB, b.DB)
		if ctx.Err() != nil {
			return
		}
		s.SetState(func() {
			p := plan.New(a.PlanInput(), b.PlanInput(), changes)
			sess.Plan, sess.Decided = p, make([]bool, len(changes))
			s.model = NewModel(p, sess.Decided)
		})
	})
}

// Dispose waits for a comparison that is still running: it reads the databases,
// which the session wipes next.
func (s *screenState) Dispose() { s.work.Wait() }

// Model is the model of the list, nil while the databases are being compared. It is
// for tests.
func (s *screenState) Model() *Model { return s.model }

// refresh rebuilds the screen after the model changed.
func (s *screenState) refresh() { s.SetState(func() {}) }

func (s *screenState) Build(bc widgets.BuildContext) widgets.Widget {
	th := kit.ThemeOf(bc)
	w := s.Widget()
	gap := func(h float64) widgets.Widget {
		return widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(h)})
	}

	var content widgets.Widget
	switch {
	case s.model == nil && w.sess.Ready():
		content = widgets.NewCenter(kit.NewProgress("Comparing the two databases…", foundation.None[float64]()))
	case s.model == nil:
		content = widgets.NewCenter(kit.NewBanner(kit.ErrorBanner, "Unlock both databases first.", ""))
	default:
		content = widgets.NewRow([]widgets.Widget{
			widgets.NewSizedBox(NewList(s.model, s.refresh), widgets.SizedBoxOpts{Width: foundation.Some(ListWidth)}),
			widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: foundation.Some(kit.SpaceXL)}),
			widgets.NewExpanded(s.details(th)),
		}, widgets.RowOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch})
	}

	next := w.onNext
	if s.model == nil {
		next = nil
	}
	return widgets.NewPadding(painting.NewEdgeInsetsAll(kit.SpaceXL), widgets.NewColumn([]widgets.Widget{
		widgets.NewSemantics(widgets.NewText("Review the changes", widgets.TextOpts{Style: th.Title()}), widgets.SemanticsOpts{Header: foundation.Some(true)}),
		gap(kit.SpaceS),
		widgets.NewText(s.subtitle(), widgets.TextOpts{Style: th.Caption(), MaxLines: foundation.Some(1), Overflow: painting.TextOverflowEllipsis}),
		gap(kit.SpaceL),
		widgets.NewExpanded(content),
		gap(kit.SpaceL),
		widgets.NewRow([]widgets.Widget{
			kit.NewButton("Back", w.onBack),
			widgets.NewExpanded(widgets.NewPadding(painting.NewEdgeInsetsSymmetric(0, kit.SpaceL),
				widgets.NewText(s.footer(), widgets.TextOpts{Style: th.Caption(), MaxLines: foundation.Some(1), Overflow: painting.TextOverflowEllipsis}))),
			kit.NewButton("Next: write the new database", next, kit.ButtonOpts{Kind: kit.Primary}),
		}, widgets.RowOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentCenter}),
	}, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch}))
}

// subtitle names the two files.
func (s *screenState) subtitle() string {
	sess := s.Widget().sess
	if !sess.Ready() {
		return ""
	}
	return "A: " + filepath.Base(sess.A.Path) + "    B: " + filepath.Base(sess.B.Path)
}

// footer says how far the review is.
func (s *screenState) footer() string {
	if s.model == nil {
		return ""
	}
	m := s.model
	switch {
	case m.Total() == 0:
		return "The two databases hold the same."
	case m.Undecided() == 0:
		return "Every change is decided."
	}
	return plural(m.Undecided(), "change") + " not decided yet; they keep their default choice."
}

// details is the right side of the screen: the change under the cursor.
func (s *screenState) details(th kit.Theme) widgets.Widget {
	var children []widgets.Widget
	line := func(k, v string) widgets.Widget {
		return widgets.NewRow([]widgets.Widget{
			widgets.NewSizedBox(widgets.NewText(k, widgets.TextOpts{Style: th.Caption()}), widgets.SizedBoxOpts{Width: foundation.Some(110.0)}),
			widgets.NewExpanded(widgets.NewText(v, widgets.TextOpts{Style: th.Body()})),
		})
	}
	if i, ok := s.model.Current(); !ok {
		children = append(children, widgets.NewText("Select a change to see it.", widgets.TextOpts{Style: th.Caption()}))
	} else {
		c := s.model.Plan().Changes[i]
		children = append(children,
			widgets.NewText(Title(c), widgets.TextOpts{Style: th.Bold()}),
			line("Kind", KindLabel(c.Kind)),
			line("Choice", ChoiceLabel(s.model.Plan().Choices[i])),
		)
		if len(c.Fields) > 0 {
			children = append(children, line("Differs in", strings.Join(c.Fields, ", ")))
		}
		if n := s.model.SelectedCount(); n > 1 {
			children = append(children, line("Selected", strconv.Itoa(n)+" changes"))
		}
	}
	return widgets.NewContainer(widgets.NewColumn(children, widgets.ColumnOpts{
		CrossAxisAlignment: rendering.CrossAxisAlignmentStart, MainAxisSize: rendering.MainAxisSizeMin,
	}), widgets.ContainerOpts{
		Padding: painting.NewEdgeInsetsAll(kit.SpaceL),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color:        foundation.Some(th.Surface),
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(th.Border)}),
			BorderRadius: painting.NewBorderRadiusCircular(kit.Radius * 2),
		}),
		Alignment: painting.AlignmentTopLeft,
	})
}
