package write

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/glutter/desktop"
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/rendering"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/merge"
	"github.com/rm4n0s/passbank/internal/core/message"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/output"
	"github.com/rm4n0s/passbank/internal/core/plan"
	"github.com/rm4n0s/passbank/internal/core/secret"
	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/review"
	"github.com/rm4n0s/passbank/internal/gui/session"
)

// kdbxFilter is the filter of the file dialog for databases.
var kdbxFilter = desktop.FileFilter{Name: "KeePass databases", Patterns: []string{"*.kdbx", "*.KDBX"}}

// defaultName is the name that the file dialog starts with.
const defaultName = "merged.kdbx"

// maxListed is how many losses the screen lists.
const maxListed = 8

// Screen is the Write screen (SPEC §13 screen 3): where to write the new database,
// in which version, with whose credentials and encryption, what is lost in that
// version, and the Write button. It builds the result of the merge off the UI
// thread when it opens, and writes it off the UI thread with a progress bar and a
// Cancel button.
type Screen struct {
	widgets.StatefulWidget
	sess   *session.Session
	onBack func()
}

// NewScreen returns the Write screen of the session; onBack returns to Review.
func NewScreen(sess *session.Session, onBack func(), opts ...widgets.WidgetOpts) *Screen {
	return &Screen{StatefulWidget: widgets.NewStatefulWidgetBase(opts...), sess: sess, onBack: onBack}
}

// CreateState makes the state of the screen.
func (s *Screen) CreateState() widgets.State { return &screenState{} }

// built is the result of one run of merge.Apply.
type built struct {
	db  *model.Database
	err error
	// groups and entries count what the result holds.
	groups, entries int
}

// outcome is how the last Write ended.
type outcome struct {
	err     error
	path    string
	version kdbx.Version
	losses  int
}

type screenState struct {
	widgets.StateOf[*Screen]
	path, password, confirm, keyFile  *widgets.TextEditingController
	pathListener, keyListener         foundation.ListenerHandle
	passwordListener, confirmListener foundation.ListenerHandle

	// mu guards built, gen and disposed, which the goroutine that builds the result
	// sets.
	mu       sync.Mutex
	built    *built
	gen      int
	disposed bool
	work     sync.WaitGroup

	// settingsIdx is the index of the settings change in the plan, or -1.
	settingsIdx int

	// lossKey and losses cache kdbx.Losses of the result for a version.
	lossKey    string
	lossResult *model.Database
	losses     []kdbx.Loss

	writing bool
	cancel  context.CancelFunc
	outcome *outcome
	// confirming is set while the dialog about the losses is open.
	confirming bool
	// chosenPath is the path the file dialog gave: the user confirmed there that an
	// existing file is replaced.
	chosenPath string
	// err is a problem with the file dialog.
	err error
}

func (s *screenState) InitState() {
	w := s.Widget()
	out := w.sess.Output
	s.path, s.keyFile = widgets.NewTextEditingController(), widgets.NewTextEditingController()
	s.password, s.confirm = widgets.NewTextEditingController(), widgets.NewTextEditingController()
	s.path.SetText(out.Path)
	s.keyFile.SetText(out.KeyFile)
	s.pathListener = s.path.AddListener(func() {
		w.sess.Output.Path = s.path.Text()
		s.SetState(func() { s.outcome = nil })
	})
	s.keyListener = s.keyFile.AddListener(func() {
		w.sess.Output.KeyFile = s.keyFile.Text()
		s.SetState(func() { s.outcome = nil })
	})
	// Whatever changes a password (typing, a test) shows in the live checks.
	s.passwordListener = s.password.AddListener(func() { s.SetState(func() { s.outcome = nil }) })
	s.confirmListener = s.confirm.AddListener(func() { s.SetState(func() {}) })
	s.settingsIdx = -1
	if p := w.sess.Plan; p != nil {
		for i, c := range p.Changes {
			if c.Kind == diff.Settings {
				s.settingsIdx = i
			}
		}
	}
	s.build()
}

// Dispose stops what runs, wipes the result and clears what was typed.
func (s *screenState) Dispose() {
	if s.cancel != nil {
		s.cancel()
	}
	s.work.Wait()
	s.mu.Lock()
	s.disposed = true
	if s.built != nil && s.built.db != nil {
		s.built.db.Wipe()
	}
	s.built = nil
	s.mu.Unlock()
	s.path.RemoveListener(s.pathListener)
	s.keyFile.RemoveListener(s.keyListener)
	s.password.RemoveListener(s.passwordListener)
	s.confirm.RemoveListener(s.confirmListener)
	s.password.SetText("")
	s.confirm.SetText("")
	for _, c := range []*widgets.TextEditingController{s.path, s.keyFile, s.password, s.confirm} {
		c.Dispose()
	}
}

// ready says that the screen has what it needs: both databases and a plan.
func (s *screenState) ready() bool {
	sess := s.Widget().sess
	return sess.Ready() && sess.Plan != nil
}

// result is the result of the merge, or nil while it is being built or when the
// plan cannot be applied.
func (s *screenState) result() *built {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.built
}

// build runs merge.Apply off the UI thread; the result replaces the one before,
// which is wiped.
func (s *screenState) build() {
	if !s.ready() {
		return
	}
	sess := s.Widget().sess
	p := *sess.Plan
	p.Choices = slices.Clone(p.Choices)
	a, b := sess.A.DB, sess.B.DB
	s.mu.Lock()
	s.gen++
	gen := s.gen
	s.mu.Unlock()
	// Handed off through errors.Async on the UI thread, so the route of an error
	// starts at build. This stays State.Go and does not use State.GoErr on purpose:
	// the goroutine must wipe the result when the state is gone, and GoErr skips its
	// done function then, so a result built just before Dispose would stay unwiped.
	var res *model.Database
	apply := errors.Async(func() (err error) {
		res, err = merge.Apply(a, b, &p)
		return err
	})
	s.work.Add(1)
	s.Go(func(ctx context.Context) {
		defer s.work.Done()
		err := apply()
		r := &built{db: res, err: err}
		if res != nil {
			res.WalkGroups(func(g *model.Group) bool { r.groups++; r.entries += len(g.Entries); return true })
		}
		s.mu.Lock()
		if s.disposed || gen != s.gen || ctx.Err() != nil {
			s.mu.Unlock()
			if res != nil {
				res.Wipe()
			}
			return
		}
		old := s.built
		s.built = r
		s.mu.Unlock()
		if old != nil && old.db != nil {
			old.db.Wipe()
		}
		s.SetState(func() {})
	})
}

// output is the remembered choices of the screen.
func (s *screenState) out() *session.Output { return &s.Widget().sess.Output }

// version is the version the result would be written as.
func (s *screenState) version(r *built) kdbx.Version {
	var db *model.Database
	if r != nil {
		db = r.db
	}
	return LookupVersion(s.out().Version).For(db)
}

// lossesOf lists what the version cannot store, remembered for the same result and
// version.
func (s *screenState) lossesOf(r *built, v kdbx.Version) []kdbx.Loss {
	if r == nil || r.db == nil {
		return nil
	}
	key := v.String()
	if s.lossResult != r.db || s.lossKey != key {
		s.lossResult, s.lossKey, s.losses = r.db, key, kdbx.Losses(r.db, v)
	}
	return s.losses
}

// settings are the encryption settings that the screen's choices give.
func (s *screenState) settings(v kdbx.Version) kdbx.EncryptionSettings {
	sess := s.Widget().sess
	return SettingsFor(EncryptionSource(*s.out()), sess.A, sess.B, v)
}

// problem is what keeps the screen from writing, as the user reads it, or "".
func (s *screenState) problem(r *built, v kdbx.Version) string {
	switch {
	case !s.ready():
		return "Compare the two databases first."
	case r == nil:
		return "Building the result…"
	case r.err != nil:
		return message.Of(r.err, "", message.GUI)
	}
	if err := kdbx.Supports(v, s.settings(v)); err != nil {
		return message.Of(err, "", message.GUI)
	}
	if CredentialsSource(*s.out()) == CredentialsNew {
		if s.password.Text() != s.confirm.Text() {
			return "The two passwords are not the same."
		}
		if s.password.Text() == "" && strings.TrimSpace(s.keyFile.Text()) == "" {
			return "A database needs a password, a key file, or both."
		}
	}
	return ""
}

func (s *screenState) Build(bc widgets.BuildContext) widgets.Widget {
	th := kit.ThemeOf(bc)
	r := s.result()
	v := s.version(r)
	gap := func(h float64) widgets.Widget {
		return widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(h)})
	}

	body := []widgets.Widget{
		widgets.NewSemantics(widgets.NewText("Write the new database", widgets.TextOpts{Style: th.Title()}), widgets.SemanticsOpts{Header: foundation.Some(true)}),
		gap(kit.SpaceS),
		widgets.NewText("Passbank writes a new file and never changes the two databases it read.", widgets.TextOpts{Style: th.Caption()}),
		gap(kit.SpaceL),
		s.fileCard(th, r, v),
		gap(kit.SpaceM),
		s.securityCard(th),
		gap(kit.SpaceM),
	}
	body = append(body, s.checks(th, r, v, gap)...)
	body = append(body, s.summary(th, r, v), gap(kit.SpaceL), s.actions(r, v), gap(kit.SpaceM))
	if s.outcome != nil {
		body = append(body, s.outcomeBanner(), gap(kit.SpaceM))
	}

	page := widgets.NewSingleChildScrollView(widgets.NewPadding(painting.NewEdgeInsetsAll(kit.SpaceXL),
		widgets.NewColumn(body, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch})))
	if !s.confirming {
		return page
	}
	return widgets.NewStack([]widgets.Widget{page, s.lossDialog(r, v)}, widgets.StackOpts{Fit: rendering.StackFitExpand})
}

// card is a panel with a heading.
func (s *screenState) card(th kit.Theme, heading string, children ...widgets.Widget) widgets.Widget {
	items := []widgets.Widget{
		widgets.NewSemantics(widgets.NewText(heading, widgets.TextOpts{Style: kit.TextStyle(18, ui.FontWeightW700, th.Text)}), widgets.SemanticsOpts{Header: foundation.Some(true)}),
		widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(kit.SpaceM)}),
	}
	items = append(items, children...)
	return widgets.NewContainer(widgets.NewColumn(items, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch}), widgets.ContainerOpts{
		Padding: painting.NewEdgeInsetsAll(kit.SpaceL),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color:        foundation.Some(th.Surface),
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(th.Border)}),
			BorderRadius: painting.NewBorderRadiusCircular(kit.Radius * 2),
		}),
	})
}

// row is a label above a control.
func row(th kit.Theme, label string, control widgets.Widget) widgets.Widget {
	return widgets.NewPadding(painting.NewEdgeInsetsFromLTRB(0, 0, 0, kit.SpaceM), widgets.NewColumn([]widgets.Widget{
		// The control names itself; the caption is for the eyes.
		widgets.NewExcludeSemantics(widgets.NewText(label, widgets.TextOpts{Style: th.Caption()})),
		widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(kit.SpaceS)}),
		control,
	}, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStart}))
}

// fileCard is the output path and the version.
func (s *screenState) fileCard(th kit.Theme, r *built, v kdbx.Version) widgets.Widget {
	segs := make([]kit.Segment, len(Versions))
	for i, o := range Versions {
		segs[i] = kit.Segment{Label: o.Label, Value: o.Value}
	}
	busy := s.writing
	var versionNote widgets.Widget = widgets.NewSizedBox(nil)
	if opt := LookupVersion(s.out().Version); opt.Auto && r != nil && r.db != nil {
		versionNote = widgets.NewText("This is KDBX "+v.String()+": "+map[bool]string{
			true:  "the result holds data that only 4.1 stores.",
			false: "the least version that stores everything of the result.",
		}[v.Minor == 1], widgets.TextOpts{Style: th.Caption()})
	}
	var choosePath func()
	if !busy {
		choosePath = s.choosePath
	}
	return s.card(th, "The file",
		row(th, "New database", kit.NewFileField("New database", choosePath, kit.FileFieldOpts{Controller: s.path, Hint: "Where to write it", ChooseLabel: "Choose…"})),
		row(th, "Version", widgets.NewColumn([]widgets.Widget{
			kit.NewSegmented("Version", segs, LookupVersion(s.out().Version).Value, func(val string) {
				if !busy {
					s.out().Version = val
					s.SetState(func() { s.outcome = nil })
				}
			}),
			widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(kit.SpaceS)}),
			versionNote,
		}, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStart})),
	)
}

// securityCard is the settings source, the credentials and the encryption.
func (s *screenState) securityCard(th kit.Theme) widgets.Widget {
	sess := s.Widget().sess
	busy := s.writing
	pick := func(set func(string)) func(string) {
		return func(val string) {
			if busy {
				return
			}
			set(val)
			s.SetState(func() { s.outcome = nil })
		}
	}
	var items []widgets.Widget
	if s.settingsIdx >= 0 {
		cur := string(sess.Plan.Choices[s.settingsIdx])
		items = append(items, row(th, "Database settings (name, description, history limits, ...)", kit.NewSegmented("Database settings from", []kit.Segment{
			{Label: "From A", Value: string(plan.TakeA)}, {Label: "From B", Value: string(plan.TakeB)},
		}, cur, func(val string) {
			if busy {
				return
			}
			sess.Plan.Choices[s.settingsIdx] = plan.Choice(val)
			if sess.Decided != nil {
				sess.Decided[s.settingsIdx] = true
			}
			s.SetState(func() { s.outcome = nil })
			s.build()
		})))
	}
	creds := CredentialsSource(*s.out())
	items = append(items, row(th, "Password and key file of the new database", kit.NewSegmented("Credentials", []kit.Segment{
		{Label: "As A", Value: SourceA}, {Label: "As B", Value: SourceB}, {Label: "New", Value: CredentialsNew},
	}, creds, pick(func(val string) { s.out().Credentials = val }))))
	if creds == CredentialsNew {
		items = append(items,
			row(th, "New password", kit.NewPasswordField("New password", kit.TextFieldOpts{Controller: s.password, Hint: "Password"})),
			row(th, "Repeat the password", kit.NewPasswordField("Repeat the new password", kit.TextFieldOpts{Controller: s.confirm, Hint: "Password again"})),
			row(th, "Key file (optional)", kit.NewFileField("Key file of the new database", s.chooseKeyFile, kit.FileFieldOpts{Controller: s.keyFile, Hint: "Key file (optional)", ChooseLabel: "Choose…"})),
		)
	}
	items = append(items, row(th, "Encryption (cipher and key derivation)", kit.NewSegmented("Encryption", []kit.Segment{
		{Label: "As A", Value: SourceA}, {Label: "As B", Value: SourceB}, {Label: "Defaults of the version", Value: EncryptionDefault},
	}, EncryptionSource(*s.out()), pick(func(val string) { s.out().Encryption = val }))))
	return s.card(th, "Settings and security", items...)
}

// checks are the live warnings: a problem that blocks writing, and what the
// version cannot store.
func (s *screenState) checks(th kit.Theme, r *built, v kdbx.Version, gap func(float64) widgets.Widget) []widgets.Widget {
	var out []widgets.Widget
	if p := s.problem(r, v); p != "" && r != nil {
		out = append(out, kit.NewBanner(kit.ErrorBanner, p, ""), gap(kit.SpaceM))
	} else if p != "" && !s.ready() {
		out = append(out, kit.NewBanner(kit.ErrorBanner, p, ""), gap(kit.SpaceM))
	}
	if s.err != nil {
		out = append(out, kit.NewBanner(kit.ErrorBanner, message.Of(s.err, "", message.GUI), ""), gap(kit.SpaceM))
	}
	if losses := s.lossesOf(r, v); len(losses) > 0 {
		out = append(out, kit.NewBanner(kit.WarningBanner,
			"KDBX "+v.String()+" cannot store everything in the result: "+strconv.Itoa(len(losses))+" "+plural(len(losses), "thing", "things")+" would be lost. Writing asks you to confirm.",
			lossText(losses, maxListed)), gap(kit.SpaceM))
	}
	return out
}

// lossText lists the losses, at most max of them.
func lossText(losses []kdbx.Loss, max int) string {
	var lines []string
	for i, l := range losses {
		if i == max {
			lines = append(lines, "… and "+strconv.Itoa(len(losses)-max)+" more")
			break
		}
		lines = append(lines, l.Item+": "+l.What)
	}
	return strings.Join(lines, "\n")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// summary says what the result will contain.
func (s *screenState) summary(th kit.Theme, r *built, v kdbx.Version) widgets.Widget {
	sess := s.Widget().sess
	var lines []widgets.Widget
	text := func(t string, style *painting.TextStyle) widgets.Widget {
		return widgets.NewText(t, widgets.TextOpts{Style: style})
	}
	if p := sess.Plan; p != nil {
		lines = append(lines, text(strconv.Itoa(len(p.Changes))+" "+plural(len(p.Changes), "change", "changes")+" between A and B:", th.Bold()))
		for _, k := range diff.Kinds {
			total := 0
			counts := map[plan.Choice]int{}
			for i, c := range p.Changes {
				if c.Kind == k {
					total++
					counts[p.Choices[i]]++
				}
			}
			if total == 0 {
				continue
			}
			var parts []string
			for _, ch := range plan.Allowed(k) {
				if counts[ch] > 0 {
					parts = append(parts, review.ChoiceLabel(ch)+" "+strconv.Itoa(counts[ch]))
				}
			}
			lines = append(lines, text(review.KindLabel(k)+": "+strconv.Itoa(total)+" ("+strings.Join(parts, ", ")+")", th.Body()))
		}
	}
	if r != nil && r.db != nil {
		lines = append(lines, widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(kit.SpaceS)}),
			text("The new database holds "+strconv.Itoa(r.groups)+" "+plural(r.groups, "group", "groups")+" and "+strconv.Itoa(r.entries)+" "+plural(r.entries, "entry", "entries")+", as KDBX "+v.String()+".", th.Bold()))
	}
	return s.card(th, "What the new database will contain", lines...)
}

// actions are the buttons, or the progress while writing.
func (s *screenState) actions(r *built, v kdbx.Version) widgets.Widget {
	w := s.Widget()
	if s.writing {
		return widgets.NewRow([]widgets.Widget{
			widgets.NewExpanded(kit.NewProgress("Writing and verifying (the key derivation can take a few seconds)…", foundation.None[float64]())),
			widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: foundation.Some(kit.SpaceL)}),
			kit.NewButton("Cancel", func() {
				if s.cancel != nil {
					s.cancel()
				}
			}),
		}, widgets.RowOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentCenter})
	}
	var write func()
	if s.problem(r, v) == "" && strings.TrimSpace(s.path.Text()) != "" {
		write = s.startWrite
	}
	return widgets.NewRow([]widgets.Widget{
		kit.NewButton("Back", w.onBack),
		widgets.NewExpanded(widgets.NewSizedBox(nil)),
		kit.NewButton("Write", write, kit.ButtonOpts{Kind: kit.Primary}),
	}, widgets.RowOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentCenter})
}

// outcomeBanner is the result of the last Write.
func (s *screenState) outcomeBanner() widgets.Widget {
	o := s.outcome
	if o.err != nil {
		return kit.NewBanner(kit.ErrorBanner, "The database was not written: "+message.Of(o.err, filepath.Base(o.path), message.GUI), "")
	}
	detail := "The file was read back with the same credentials and equals the result, so it opens in KeePassXC and other clients."
	if o.losses > 0 {
		detail += " " + strconv.Itoa(o.losses) + " " + plural(o.losses, "thing was", "things were") + " left out, as you confirmed."
	}
	return kit.NewBanner(kit.SuccessBanner, "Written and verified: "+filepath.Base(o.path)+" (KDBX "+o.version.String()+").", detail)
}

// lossDialog asks to confirm writing although the version loses data.
func (s *screenState) lossDialog(r *built, v kdbx.Version) widgets.Widget {
	losses := s.lossesOf(r, v)
	body := widgets.NewText("KDBX " + v.String() + " cannot store everything in the result. These would be lost in the new file; the two databases you opened keep them.\n\n" + lossText(losses, maxListed))
	return kit.NewDialog("Write although data is lost?", body, []widgets.Widget{
		kit.NewButton("Cancel", func() { s.SetState(func() { s.confirming = false }) }),
		kit.NewButton("Write and lose it", func() {
			s.SetState(func() { s.confirming = false })
			s.begin(len(losses))
		}, kit.ButtonOpts{Kind: kit.Danger}),
	}, 560)
}

// choosePath opens the save dialog. Its own confirmation of replacing a file counts
// as the user's consent.
func (s *screenState) choosePath() {
	bc := s.Context()
	opts := desktop.SaveFileOpts{Title: "Write the new database", CurrentName: defaultName, Filters: []desktop.FileFilter{kdbxFilter}}
	if cur := strings.TrimSpace(s.path.Text()); cur != "" {
		opts.CurrentFolder, opts.CurrentName = filepath.Dir(cur), filepath.Base(cur)
	}
	// GoErr hands the dialog off through errors.Async on the UI thread, so the route
	// of its error starts at choosePath.
	var res desktop.FilesResult
	s.GoErr(func(ctx context.Context) (err error) {
		res, err = desktop.SaveFile(ctx, bc, opts)
		return err
	}, func(err error) {
		s.SetState(func() {
			switch {
			case err != nil:
				s.err = err
			case !res.Cancelled && len(res.Paths) > 0:
				s.err = nil
				s.chosenPath = res.Paths[0]
				s.path.SetText(res.Paths[0])
			}
		})
	})
}

// chooseKeyFile opens the dialog for the key file of the new credentials.
func (s *screenState) chooseKeyFile() {
	bc := s.Context()
	// GoErr hands the dialog off through errors.Async on the UI thread, so the route
	// of its error starts at chooseKeyFile.
	var res desktop.FilesResult
	s.GoErr(func(ctx context.Context) (err error) {
		res, err = desktop.OpenFiles(ctx, bc, desktop.OpenFilesOpts{Title: "Choose the key file of the new database"})
		return err
	}, func(err error) {
		s.SetState(func() {
			switch {
			case err != nil:
				s.err = err
			case !res.Cancelled && len(res.Paths) > 0:
				s.err = nil
				s.keyFile.SetText(res.Paths[0])
			}
		})
	})
}

// startWrite is the Write button: it asks for confirmation when the version loses
// data, and writes.
func (s *screenState) startWrite() {
	r := s.result()
	v := s.version(r)
	if s.writing || s.problem(r, v) != "" {
		return
	}
	if n := len(s.lossesOf(r, v)); n > 0 {
		s.SetState(func() { s.confirming = true })
		return
	}
	s.begin(0)
}

// begin writes the file off the UI thread; losses is how many losses the user
// accepted.
func (s *screenState) begin(losses int) {
	sess := s.Widget().sess
	r := s.result()
	if r == nil || r.db == nil {
		return
	}
	v := s.version(r)
	settings := s.settings(v)
	path := strings.TrimSpace(s.path.Text())
	opts := output.Options{AllowLoss: losses > 0, Overwrite: path == s.chosenPath, Inputs: []string{sess.A.Path, sess.B.Path}}

	source := CredentialsSource(*s.out())
	var creds crypto.Credentials
	var keyPath string
	switch source {
	case SourceA:
		creds = sess.A.Creds
	case SourceB:
		creds = sess.B.Creds
	default:
		creds.Password = secret.New([]byte(s.password.Text()))
		creds.HasPassword = creds.Password.Len() > 0
		keyPath = strings.TrimSpace(s.keyFile.Text())
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.writing, s.outcome, s.err = cancel, true, nil, nil
	s.SetState(func() {})
	// GoErr hands the write off through errors.Async on the UI thread, so the route of
	// an error starts at begin. work.Done and cancel run when the write ends, also
	// when the state was disposed and done is skipped.
	s.work.Add(1)
	s.GoErr(func(parent context.Context) error {
		defer s.work.Done()
		defer cancel()
		stop := context.AfterFunc(parent, cancel)
		defer stop()
		if source == CredentialsNew {
			defer func() { creds.Wipe() }() // the credentials are made for this write
			if keyPath != "" {
				key, err := crypto.ReadKeyFile(ctx, keyPath)
				if err != nil {
					return err
				}
				creds.HasKeyFile, creds.KeyFile = true, key
			}
		}
		return output.Write(ctx, path, r.db, v, settings, creds, opts)
	}, func(err error) {
		s.SetState(func() {
			s.writing, s.cancel = false, nil
			if cancelled(err) {
				return
			}
			s.outcome = &outcome{err: err, path: path, version: v, losses: losses}
			if err == nil {
				s.chosenPath = "" // the next write to the same path asks again
			}
		})
	})
}

// cancelled says whether err is the user's Cancel.
func cancelled(err error) bool {
	appErr, ok := errors.FromError(err)
	return ok && (appErr.Tag == "WriteCancelled" || appErr.Tag == "KdfCancelled" || appErr.Tag == "ReadCancelled")
}
