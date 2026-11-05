package review

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/rm4n0s/glutter/desktop"
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/rendering"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/message"
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
	// busy is set while a plan file is being chosen, read or written.
	busy bool
	// note is the last result of Load plan or Save plan, shown above the list; nil
	// when there is none.
	note *note
}

// note is a result that the screen shows in a banner.
type note struct {
	kind kit.BannerKind
	text string
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

	compact := isCompact(bc)
	var content widgets.Widget
	switch {
	case s.model == nil && w.sess.Ready():
		content = widgets.NewCenter(kit.NewProgress("Comparing the two databases…", foundation.None[float64]()))
	case s.model == nil:
		content = widgets.NewCenter(kit.NewBanner(kit.ErrorBanner, "Unlock both databases first.", ""))
	case compact:
		// The list above the details, each of a height of its own; the page scrolls.
		content = widgets.NewColumn([]widgets.Widget{
			widgets.NewSizedBox(NewList(s.model, s.refresh), widgets.SizedBoxOpts{Height: foundation.Some(kit.Scaled(bc, compactListHeight))}),
			gap(kit.SpaceL),
			widgets.NewSizedBox(s.details(th), widgets.SizedBoxOpts{Height: foundation.Some(kit.Scaled(bc, compactDetailsHeight))}),
		}, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch})
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
	var loadPlan, savePlan func()
	if s.model != nil && !s.busy {
		loadPlan, savePlan = s.loadPlan, s.savePlan
	}
	column := []widgets.Widget{
		widgets.NewRow([]widgets.Widget{
			widgets.NewExpanded(widgets.NewSemantics(widgets.NewText("Review the changes", widgets.TextOpts{Style: th.Title()}), widgets.SemanticsOpts{Header: foundation.Some(true)})),
			kit.NewButton("Load plan…", loadPlan),
			widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: foundation.Some(kit.SpaceM)}),
			kit.NewButton("Save plan…", savePlan),
		}, widgets.RowOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentCenter}),
		gap(kit.SpaceS),
		widgets.NewText(s.subtitle(), widgets.TextOpts{Style: th.Caption(), MaxLines: foundation.Some(1), Overflow: painting.TextOverflowEllipsis}),
		gap(kit.SpaceL),
	}
	column = append(column, s.banners(gap)...)
	footer := widgets.NewRow([]widgets.Widget{
		kit.NewButton("Back", w.onBack),
		widgets.NewExpanded(widgets.NewPadding(painting.NewEdgeInsetsSymmetric(0, kit.SpaceL),
			widgets.NewText(s.footer(), widgets.TextOpts{Style: th.Caption(), MaxLines: foundation.Some(1), Overflow: painting.TextOverflowEllipsis}))),
		kit.NewButton("Next: write the new database", next, kit.ButtonOpts{Kind: kit.Primary}),
	}, widgets.RowOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentCenter})
	stretch := widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch}
	if compact {
		// Everything but the buttons at the foot scrolls.
		column = append(column, content)
		return widgets.NewPadding(painting.NewEdgeInsetsAll(kit.SpaceXL), widgets.NewColumn([]widgets.Widget{
			widgets.NewExpanded(widgets.NewSingleChildScrollView(widgets.NewColumn(column, stretch))),
			gap(kit.SpaceL),
			footer,
		}, stretch))
	}
	column = append(column, widgets.NewExpanded(content), gap(kit.SpaceL), footer)
	return widgets.NewPadding(painting.NewEdgeInsetsAll(kit.SpaceXL), widgets.NewColumn(column, stretch))
}

// The heights of the list and of the details when the screen is compact, before the
// text size is applied.
const (
	compactListHeight    = 420.0
	compactDetailsHeight = 520.0
)

// isCompact says whether the window or the text is too small for the list and the
// details side by side with the page fixed: the page then scrolls, with the list
// above the details.
func isCompact(bc widgets.BuildContext) bool {
	size := widgets.MediaQueryOf(bc).Size
	return kit.Scaled(bc, 1) > 1.25 || size.Width < 1000 || size.Height < 700
}

// banners are what needs the user's attention above the list: the result of the
// last plan file action, and every group that the plan drops while it keeps items
// in it, each with the fix.
func (s *screenState) banners(gap func(float64) widgets.Widget) []widgets.Widget {
	var out []widgets.Widget
	if s.note != nil {
		out = append(out, kit.NewBanner(s.note.kind, s.note.text, ""), gap(kit.SpaceM))
	}
	if s.model == nil {
		return out
	}
	conflicts := s.model.GroupConflicts()
	for i, c := range conflicts {
		if i == maxConflictBanners {
			out = append(out, kit.NewBanner(kit.WarningBanner, plural(len(conflicts)-i, "more group")+" are dropped while items in them are kept.", ""), gap(kit.SpaceM))
			break
		}
		group := c.Group
		out = append(out,
			kit.NewBanner(kit.WarningBanner, "The group "+quote(Title(s.model.Plan().Changes[group]))+" is dropped, but "+plural(len(c.Items), "item")+" in it "+
				verb(len(c.Items))+" kept. The new database cannot be written like this.", ""),
			gap(kit.SpaceS),
			widgets.NewRow([]widgets.Widget{kit.NewButton("Drop everything in this group", func() {
				s.model.DropEverythingIn(group)
				s.refresh()
			})}),
			gap(kit.SpaceM))
	}
	return out
}

// maxConflictBanners is how many dropped groups get their own banner.
const maxConflictBanners = 3

func verb(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func quote(s string) string { return "\u201c" + s + "\u201d" }

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

// details is the right side of the screen: the change under the cursor, with the
// buttons for its choice and the table of its fields.
func (s *screenState) details(th kit.Theme) widgets.Widget {
	m := s.model
	sess := s.Widget().sess
	var children []widgets.Widget
	gap := func(h float64) widgets.Widget {
		return widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(h)})
	}
	i, ok := m.Current()
	if !ok {
		children = append(children, widgets.NewText("Select a change to see it.", widgets.TextOpts{Style: th.Caption()}))
	} else {
		p := m.Plan()
		c := p.Changes[i]
		d := DetailsOf(c, sess.A.DB, sess.B.DB)

		heading := KindLabel(c.Kind)
		if n := m.SelectedCount(); n > 1 {
			heading += "; " + strconv.Itoa(n) + " changes are selected, the buttons set them all"
		}
		segs := make([]kit.Segment, 0, len(choiceButtons))
		for _, ch := range choiceButtons {
			segs = append(segs, kit.Segment{Label: ChoiceLabel(ch), Value: string(ch), Disabled: !m.CanChoose(ch)})
		}
		children = append(children,
			widgets.NewText(Title(c), widgets.TextOpts{Style: th.Bold(), MaxLines: foundation.Some(2), Overflow: painting.TextOverflowEllipsis}),
			widgets.NewText(heading, widgets.TextOpts{Style: th.Caption()}),
			gap(kit.SpaceM),
			kit.NewSegmented("Choice", segs, string(p.Choices[i]), func(v string) {
				m.ChooseValue(plan.Choice(v))
				s.refresh()
			}),
			gap(kit.SpaceM),
			widgets.NewExpanded(widgets.NewSingleChildScrollView(widgets.NewColumn([]widgets.Widget{
				// A value that was revealed is hidden again when another change is shown.
				kit.NewFieldTable(d.Rows, "A: "+filepath.Base(sess.A.Path), "B: "+filepath.Base(sess.B.Path), i),
				s.also(th, d.Also),
			}, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch}))),
		)
	}
	return widgets.NewContainer(widgets.NewColumn(children, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch}), widgets.ContainerOpts{
		Padding: painting.NewEdgeInsetsAll(kit.SpaceL),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color:        foundation.Some(th.Surface),
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(th.Border)}),
			BorderRadius: painting.NewBorderRadiusCircular(kit.Radius * 2),
		}),
	})
}

// choiceButtons are the choices of the buttons, in order.
var choiceButtons = []plan.Choice{plan.TakeA, plan.TakeB, plan.Both, plan.Keep, plan.Drop}

// also names the differences that the table has no row for.
func (s *screenState) also(th kit.Theme, names []string) widgets.Widget {
	if len(names) == 0 {
		return widgets.NewSizedBox(nil)
	}
	return widgets.NewPadding(painting.NewEdgeInsetsSymmetric(kit.SpaceM, 0),
		widgets.NewText("Also differs: "+joinNames(names)+".", widgets.TextOpts{Style: th.Caption()}))
}

func joinNames(names []string) string { return strings.Join(names, ", ") }

// planFileName is the name the dialog of Save plan starts with.
const planFileName = "passbank-plan.txt"

// loadPlan asks for a plan file and takes its choices.
func (s *screenState) loadPlan() {
	m, sess, bc := s.model, s.Widget().sess, s.Context()
	s.SetState(func() { s.busy, s.note = true, nil })
	s.Go(func(ctx context.Context) {
		res, err := desktop.OpenFiles(ctx, bc, desktop.OpenFilesOpts{Title: "Load a plan"})
		if err == nil && (res.Cancelled || len(res.Paths) == 0) {
			s.SetState(func() { s.busy = false })
			return
		}
		var loaded *plan.Plan
		path := ""
		if err == nil {
			path = res.Paths[0]
			loaded, err = plan.ReadFile(ctx, path, sess.A.PlanInput(), sess.B.PlanInput(), m.Plan().Changes)
		}
		s.SetState(func() {
			s.busy = false
			if err != nil {
				s.note = &note{kit.ErrorBanner, "The plan was not loaded: " + message.Of(err, filepath.Base(path), message.GUI)}
				return
			}
			m.Replace(loaded.Choices)
			s.note = &note{kit.SuccessBanner, "Loaded the choices of " + filepath.Base(path) + "."}
		})
	})
}

// savePlan asks where to save the plan and writes it. The dialog asks the user to
// confirm replacing a file, which counts as their consent.
func (s *screenState) savePlan() {
	m, sess, bc := s.model, s.Widget().sess, s.Context()
	// The text is made here, on the UI thread, which owns the plan.
	var text bytes.Buffer
	if err := plan.Write(&text, m.Plan(), plan.WriteOptions{}); err != nil {
		s.SetState(func() {
			s.note = &note{kit.ErrorBanner, "The plan was not saved: " + message.Of(err, planFileName, message.GUI)}
		})
		return
	}
	data := text.Bytes()
	a, b := sess.A.Path, sess.B.Path
	s.SetState(func() { s.busy, s.note = true, nil })
	s.Go(func(ctx context.Context) {
		res, err := desktop.SaveFile(ctx, bc, desktop.SaveFileOpts{Title: "Save the plan", CurrentName: planFileName})
		if err == nil && (res.Cancelled || len(res.Paths) == 0) {
			s.SetState(func() { s.busy = false })
			return
		}
		path := planFileName
		if err == nil {
			path = res.Paths[0]
			if err = plan.CheckPath(path, a, b); err == nil {
				err = plan.WriteFile(ctx, path, data, true)
			}
		}
		s.SetState(func() {
			s.busy = false
			if err != nil {
				s.note = &note{kit.ErrorBanner, "The plan was not saved: " + message.Of(err, filepath.Base(path), message.GUI)}
				return
			}
			s.note = &note{kit.SuccessBanner, "Saved the plan to " + filepath.Base(path) + "."}
		})
	})
}
