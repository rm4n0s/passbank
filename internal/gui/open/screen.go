// Package open is the Open screen of the GUI (SPEC §13, screen 1): two panels, A
// and B, in which the user chooses a database, gives its password and key file and
// unlocks it, and the buttons to swap the two and to compare them.
package open

import (
	"context"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/desktop"
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/rendering"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/core/message"
	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/session"
)

// Screen is the Open screen. It unlocks into sess and calls onCompare when the user
// presses Compare with both sides unlocked.
type Screen struct {
	widgets.StatefulWidget
	sess      *session.Session
	onCompare func()
}

// NewScreen returns the Open screen of the session.
func NewScreen(sess *session.Session, onCompare func(), opts ...widgets.WidgetOpts) *Screen {
	return &Screen{StatefulWidget: widgets.NewStatefulWidgetBase(opts...), sess: sess, onCompare: onCompare}
}

// CreateState makes the state of the screen.
func (s *Screen) CreateState() widgets.State { return &screenState{} }

// panel is the state of one side of the screen.
type panel struct {
	name     string // "A" or "B"
	path     *widgets.TextEditingController
	key      *widgets.TextEditingController
	password *widgets.TextEditingController
	// pathListener is the registration that notices a change of the path.
	pathListener foundation.ListenerHandle
	err          error
	unlocking    bool
	cancel       context.CancelFunc
}

type screenState struct {
	widgets.StateOf[*Screen]
	panels [2]*panel
}

func (s *screenState) InitState() {
	for i, name := range []string{"A", "B"} {
		s.panels[i] = &panel{
			name: name, path: widgets.NewTextEditingController(), key: widgets.NewTextEditingController(),
			password: widgets.NewTextEditingController(),
		}
	}
	// A database that is already unlocked (the screen was built again, or a debug flag
	// preloaded it) shows its path.
	for i := range s.panels {
		i := i
		if src := s.source(i); src != nil {
			s.panels[i].path.SetText(src.Path)
		}
		// Whatever changes the path (typing, the dialog, a drop, a swap) locks a side
		// whose database is not that file.
		s.panels[i].pathListener = s.panels[i].path.AddListener(func() { s.SetState(func() { s.pathChanged(i) }) })
	}
}

func (s *screenState) Dispose() {
	for _, p := range s.panels {
		if p.cancel != nil {
			p.cancel()
		}
		p.path.RemoveListener(p.pathListener)
		p.path.Dispose()
		p.key.Dispose()
		// The password is typed by the user; clear it before the controller goes.
		p.password.SetText("")
		p.password.Dispose()
	}
}

// source is the unlocked database of a side, or nil.
func (s *screenState) source(i int) *session.Source {
	if i == 0 {
		return s.Widget().sess.A
	}
	return s.Widget().sess.B
}

func (s *screenState) setSource(i int, src *session.Source) {
	if i == 0 {
		s.Widget().sess.SetA(src)
	} else {
		s.Widget().sess.SetB(src)
	}
}

// kdbxFilter is the filter of the file dialog for databases.
var kdbxFilter = desktop.FileFilter{Name: "KeePass databases", Patterns: []string{"*.kdbx", "*.KDBX"}}

// choose opens the file dialog for the path (or the key file) of a panel; the
// answer is put in the field. The dialog runs off the UI thread.
func (s *screenState) choose(i int, keyFile bool) {
	p := s.panels[i]
	bc := s.Context()
	opts := desktop.OpenFilesOpts{Title: "Choose database " + p.name}
	field := p.path
	if keyFile {
		opts = desktop.OpenFilesOpts{Title: "Choose the key file of database " + p.name}
		field = p.key
	} else {
		opts.Filters = []desktop.FileFilter{kdbxFilter}
	}
	s.Go(func(ctx context.Context) {
		res, err := desktop.OpenFiles(ctx, bc, opts)
		s.SetState(func() {
			switch {
			case err != nil:
				p.err = err
			case !res.Cancelled && len(res.Paths) > 0:
				field.SetText(res.Paths[0])
				p.err = nil
			}
		})
	})
}

// pathChanged locks a side again when its path is no longer the file that was
// unlocked: what is shown must be what is open.
func (s *screenState) pathChanged(i int) {
	if src := s.source(i); src != nil && src.Path != s.panels[i].path.Text() {
		s.setSource(i, nil)
	}
}

// unlock reads and decrypts the database of a side off the UI thread. The password
// is taken from the field and wiped when it has been used; a failure leaves it so
// that the user can correct it.
func (s *screenState) unlock(i int) {
	p := s.panels[i]
	if p.unlocking {
		return
	}
	path, keyPath := strings.TrimSpace(p.path.Text()), strings.TrimSpace(p.key.Text())
	password := secret.New([]byte(p.password.Text()))
	ctx, cancel := context.WithCancel(context.Background())
	p.unlocking, p.cancel, p.err = true, cancel, nil
	s.Go(func(parent context.Context) {
		defer cancel()
		stop := context.AfterFunc(parent, cancel)
		defer stop()
		src, err := session.Unlock(ctx, path, password, keyPath)
		s.SetState(func() {
			p.unlocking, p.cancel = false, nil
			if err != nil {
				if isCancelled(err) {
					return
				}
				p.err = err
				return
			}
			s.setSource(i, src)
			p.password.SetText("") // the secret is in the session now
		})
	})
}

func isCancelled(err error) bool {
	appErr, ok := errors.FromError(err)
	return ok && (appErr.Tag == "ReadCancelled" || appErr.Tag == "KdfCancelled")
}

// swap exchanges the two sides: the databases and what is typed in the panels.
func (s *screenState) swap() {
	s.Widget().sess.Swap()
	a, b := s.panels[0], s.panels[1]
	// The fields keep their controllers; their texts change places.
	for _, pair := range [][2]*widgets.TextEditingController{{a.path, b.path}, {a.key, b.key}, {a.password, b.password}} {
		x, y := pair[0].Text(), pair[1].Text()
		pair[0].SetText(y)
		pair[1].SetText(x)
	}
	a.err, b.err = b.err, a.err
}

func (s *screenState) Build(bc widgets.BuildContext) widgets.Widget {
	th := kit.ThemeOf(bc)
	w := s.Widget()
	var compare func()
	if w.sess.Ready() && w.onCompare != nil {
		compare = w.onCompare
	}
	gap := func(h float64) widgets.Widget {
		return widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(h)})
	}
	panels := widgets.NewRow([]widgets.Widget{
		widgets.NewExpanded(s.panelView(bc, 0)),
		widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: foundation.Some(kit.SpaceXL)}),
		widgets.NewExpanded(s.panelView(bc, 1)),
	}, widgets.RowOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStart})
	body := widgets.NewColumn([]widgets.Widget{
		widgets.NewSemantics(widgets.NewText("Merge two KeePass databases", widgets.TextOpts{Style: th.Title()}),
			widgets.SemanticsOpts{Header: foundation.Some(true)}),
		gap(kit.SpaceS),
		widgets.NewText("Unlock both, then compare. Passbank writes a new database and never changes these two.", widgets.TextOpts{Style: th.Caption()}),
		gap(kit.SpaceL),
		panels,
		gap(kit.SpaceL),
		widgets.NewRow([]widgets.Widget{
			kit.NewButton("Swap A and B", s.swapAction()),
			widgets.NewExpanded(widgets.NewSizedBox(nil)),
			kit.NewButton("Compare", compare, kit.ButtonOpts{Kind: kit.Primary}),
		}),
	}, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch})
	return widgets.NewSingleChildScrollView(widgets.NewPadding(painting.NewEdgeInsetsAll(kit.SpaceXL), body))
}

// swapAction is the action of the Swap button: none while a side is being unlocked.
func (s *screenState) swapAction() func() {
	if s.panels[0].unlocking || s.panels[1].unlocking {
		return nil
	}
	return func() { s.SetState(s.swap) }
}

func (s *screenState) panelView(bc widgets.BuildContext, i int) widgets.Widget {
	th := kit.ThemeOf(bc)
	p := s.panels[i]
	src := s.source(i)
	gap := func(h float64) widgets.Widget {
		return widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(h)})
	}

	children := []widgets.Widget{
		widgets.NewSemantics(widgets.NewText("Database "+p.name, widgets.TextOpts{Style: TextH2(th)}), widgets.SemanticsOpts{Header: foundation.Some(true)}),
		gap(kit.SpaceM),
		kit.NewFileField("Database "+p.name+" file", func() { s.choose(i, false) }, kit.FileFieldOpts{
			Controller: p.path, Hint: "A .kdbx file", ChooseLabel: "Choose…",
		}),
		gap(kit.SpaceM),
		kit.NewPasswordField("Password of database "+p.name, kit.TextFieldOpts{Controller: p.password, Hint: "Password", OnSubmitted: func(string) { s.unlock(i) }}),
		gap(kit.SpaceM),
		kit.NewFileField("Key file of database "+p.name, func() { s.choose(i, true) }, kit.FileFieldOpts{Controller: p.key, Hint: "Key file (optional)", ChooseLabel: "Choose…"}),
		gap(kit.SpaceL),
	}
	switch {
	case p.unlocking:
		children = append(children,
			kit.NewProgress("Unlocking "+p.name+" (the key derivation can take a few seconds)…", foundation.None[float64]()),
			gap(kit.SpaceM),
			kit.NewButton("Cancel", func() {
				if p.cancel != nil {
					p.cancel()
				}
			}))
	default:
		label := "Unlock"
		if src != nil {
			label = "Unlock again"
		}
		children = append(children, widgets.NewRow([]widgets.Widget{kit.NewButton(label, func() { s.unlock(i) },
			kit.ButtonOpts{Kind: kit.Primary, SemanticsLabel: label + " database " + p.name})}))
	}
	if p.err != nil {
		children = append(children, gap(kit.SpaceM), kit.NewBanner(kit.ErrorBanner, message.Of(p.err, filepath.Base(p.path.Text()), message.GUI), ""))
	}
	if src != nil && !p.unlocking {
		children = append(children, gap(kit.SpaceM), summary(th, src))
	}
	card := widgets.NewContainer(widgets.NewColumn(children, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch}), widgets.ContainerOpts{
		Padding: painting.NewEdgeInsetsAll(kit.SpaceL),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color:        foundation.Some(th.Surface),
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(th.Border)}),
			BorderRadius: painting.NewBorderRadiusCircular(kit.Radius * 2),
		}),
	})
	// A file dropped on the panel sets its path. Each panel is its own traversal
	// group, so that Tab visits all of A's controls and then all of B's, instead of
	// going across the two panels row by row.
	return widgets.NewFocusTraversalGroup(widgets.NewDragTarget(func(widgets.BuildContext, []foundation.Opt[*widgets.ExternalDragData], []any) widgets.Widget {
		return card
	}, widgets.DragTargetOpts[*widgets.ExternalDragData]{
		OnWillAcceptWithDetails: func(d widgets.DragTargetDetails[*widgets.ExternalDragData]) bool {
			return d.Data != nil && d.Data.HasType("text/uri-list")
		},
		OnAcceptWithDetails: func(d widgets.DragTargetDetails[*widgets.ExternalDragData]) {
			for _, uri := range d.Data.URIs {
				if path, ok := pathOfURI(uri); ok {
					s.SetState(func() {
						p.path.SetText(path)
						p.err = nil
					})
					return
				}
			}
		},
	}))
}

// TextH2 is the style of a panel's heading.
func TextH2(th kit.Theme) *painting.TextStyle { return kit.TextStyle(18, ui.FontWeightW700, th.Text) }

// pathOfURI is the path of a file: URI.
func pathOfURI(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "file" || u.Path == "" {
		return "", false
	}
	return u.Path, true
}

// count is "1 group" or "2 groups"; plural is the plural when it is not the word with an s.
func count(n int, singular string, plural ...string) string {
	if n == 1 {
		return "1 " + singular
	}
	if len(plural) > 0 {
		return strconv.Itoa(n) + " " + plural[0]
	}
	return strconv.Itoa(n) + " " + singular + "s"
}

// summary shows how an unlocked file is stored and what it holds.
func summary(th kit.Theme, src *session.Source) widgets.Widget {
	info := src.Info
	line := func(k, v string) widgets.Widget {
		return widgets.NewRow([]widgets.Widget{
			widgets.NewSizedBox(widgets.NewText(k, widgets.TextOpts{Style: th.Caption()}), widgets.SizedBoxOpts{Width: foundation.Some(110.0)}),
			widgets.NewExpanded(widgets.NewText(v, widgets.TextOpts{Style: th.Body()})),
		})
	}
	return widgets.NewSemantics(widgets.NewContainer(widgets.NewColumn([]widgets.Widget{
		widgets.NewText("Unlocked", widgets.TextOpts{Style: kit.TextStyle(kit.SizeBody, ui.FontWeightW700, th.Success)}),
		line("Version", "KDBX "+info.Version.String()),
		line("Cipher", info.Cipher),
		line("Key derivation", info.KDF.Name()),
		line("Contents", count(info.Groups, "group")+", "+count(info.Entries, "entry", "entries")+", "+count(info.Attachments, "attachment")+", "+count(info.HistoryItems, "history item")),
	}, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStart}), widgets.ContainerOpts{
		Padding: painting.NewEdgeInsetsAll(kit.SpaceM),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color: foundation.Some(th.SuccessSurface), BorderRadius: painting.NewBorderRadiusCircular(kit.Radius),
		}),
	}), widgets.SemanticsOpts{Container: true, LiveRegion: foundation.Some(true)})
}
