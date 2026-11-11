package kit_test

import (
	"testing"

	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/rendering"
	"github.com/rm4n0s/glutter/services"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"

	"github.com/rm4n0s/passbank/internal/gui/kit"
	"github.com/rm4n0s/passbank/internal/gui/testutil"
)

func gap(h float64) widgets.Widget {
	return widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: some(h)})
}

func TestTextField(t *testing.T) {
	var changed, submitted string
	c := widgets.NewTextEditingController()
	ts := pump(t, kit.LightTheme(), ui.Size{Width: 400, Height: 100}, kit.NewTextField("Key file", kit.TextFieldOpts{
		Controller: c, Hint: "Path of the key file", OnChanged: func(s string) { changed = s }, OnSubmitted: func(s string) { submitted = s },
	}))
	if !ts.Exists(gluttertest.ByText("Path of the key file")) {
		t.Error("the hint is not shown while the field is empty")
	}
	ts.EnterText(gluttertest.ByType[*widgets.EditableText](), "/home/me/key.keyx")
	ts.PumpAndSettle()
	if changed != "/home/me/key.keyx" || c.Text() != "/home/me/key.keyx" {
		t.Errorf("OnChanged = %q, controller = %q", changed, c.Text())
	}
	if ts.Exists(gluttertest.ByText("Path of the key file")) {
		t.Error("the hint is still shown")
	}
	ts.SendKeyEvent(services.LogicalKeyboardKeyEnter)
	ts.PumpAndSettle()
	if submitted != "/home/me/key.keyx" {
		t.Errorf("OnSubmitted = %q", submitted)
	}
}

func TestTextFieldOwnsItsController(t *testing.T) {
	ts := pump(t, kit.LightTheme(), ui.Size{Width: 400, Height: 100}, kit.NewTextField("Name"))
	ts.EnterText(gluttertest.ByType[*widgets.EditableText](), "abc")
	ts.PumpAndSettle()
	ts.PumpWidget(widgets.NewSizedBox(nil)) // the field is disposed with its controller and focus node
	ts.PumpAndSettle()
}

func TestPasswordFieldReveal(t *testing.T) {
	ts := pump(t, kit.LightTheme(), ui.Size{Width: 400, Height: 100}, kit.NewPasswordField("Password"))
	editable := func() *widgets.EditableText {
		return gluttertest.WidgetAs[*widgets.EditableText](ts, gluttertest.ByType[*widgets.EditableText]())
	}
	if !editable().ObscureText() {
		t.Fatal("the password is not hidden")
	}
	ts.EnterText(gluttertest.ByType[*widgets.EditableText](), "s3cret")
	ts.Tap(gluttertest.ByText("Show"))
	ts.PumpAndSettle()
	if editable().ObscureText() {
		t.Error("the password is still hidden after Show")
	}
	if !ts.Exists(gluttertest.ByText("Hide")) {
		t.Error("the toggle does not say Hide")
	}
	ts.Tap(gluttertest.ByText("Hide"))
	ts.PumpAndSettle()
	if !editable().ObscureText() {
		t.Error("the password is not hidden again")
	}
}

func TestFileField(t *testing.T) {
	chosen := 0
	c := widgets.NewTextEditingController()
	ts := pump(t, kit.LightTheme(), ui.Size{Width: 500, Height: 100}, kit.NewFileField("Database A", func() { chosen++ }, kit.FileFieldOpts{Controller: c, Hint: "A .kdbx file"}))
	ts.Tap(gluttertest.ByText("Choose…"))
	if chosen != 1 {
		t.Errorf("Choose… called onChoose %d times", chosen)
	}
	c.SetText("/data/a.kdbx")
	ts.PumpAndSettle()
	if ts.Exists(gluttertest.ByText("A .kdbx file")) {
		t.Error("the hint is shown with a path")
	}
}

func TestSegmented(t *testing.T) {
	var got string
	segs := []kit.Segment{{Label: "Take A", Value: "take-a"}, {Label: "Take B", Value: "take-b"}, {Label: "Keep both", Value: "both", Disabled: true}}
	ts := pump(t, kit.LightTheme(), ui.Size{Width: 500, Height: 100}, kit.NewSegmented("Choice", segs, "take-a", func(v string) { got = v }))
	ts.Tap(gluttertest.ByText("Take B"))
	if got != "take-b" {
		t.Errorf("a tap chose %q", got)
	}
	got = ""
	ts.Tap(gluttertest.ByText("Keep both"))
	if got != "" {
		t.Errorf("a disabled segment chose %q", got)
	}
}

func TestListRowModifiers(t *testing.T) {
	var got []kit.Modifiers
	ts := pump(t, kit.LightTheme(), ui.Size{Width: 400, Height: 100}, kit.NewListRow(widgets.NewText("My bank"), "My bank", false, func(m kit.Modifiers) { got = append(got, m) }))
	ts.Tap(gluttertest.ByText("My bank"))
	ts.SendKeyDownEvent(services.LogicalKeyboardKeyShiftLeft)
	ts.Tap(gluttertest.ByText("My bank"))
	ts.SendKeyUpEvent(services.LogicalKeyboardKeyShiftLeft)
	ts.SendKeyDownEvent(services.LogicalKeyboardKeyControlLeft)
	ts.Tap(gluttertest.ByText("My bank"))
	ts.SendKeyUpEvent(services.LogicalKeyboardKeyControlLeft)
	want := []kit.Modifiers{{}, {Shift: true}, {Ctrl: true}}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("modifiers = %+v, want %+v", got, want)
	}
}

func fieldRows() []kit.FieldRow {
	return []kit.FieldRow{
		{Name: "Title", A: "My bank", B: "My bank"},
		{Name: "URL", A: "https://bank.example.com", B: "https://bank.example.org", Differs: true},
		{Name: "Password", A: "bank-password-a", B: "bank-password-b", Differs: true, Protected: true},
		{Name: "Notes", A: "", B: "new note", Differs: true, MissingA: true},
	}
}

func TestFieldTableMasksAndResets(t *testing.T) {
	table := func(key string) widgets.Widget {
		return kit.NewFieldTable(fieldRows(), "A", "B", key)
	}
	ts := pump(t, kit.LightTheme(), ui.Size{Width: 700, Height: 300}, table("one"))
	if ts.Exists(gluttertest.ByText("bank-password-a")) || !ts.Exists(gluttertest.ByText(kit.Masked)) {
		t.Fatal("the password is not masked")
	}
	ts.Tap(gluttertest.ByText("Show"))
	ts.PumpAndSettle()
	if !ts.Exists(gluttertest.ByText("bank-password-a")) || !ts.Exists(gluttertest.ByText("bank-password-b")) {
		t.Fatal("Show does not reveal both values")
	}
	// The same selection: it stays revealed.
	ts.PumpWidget(host(kit.LightTheme(), table("one")))
	ts.PumpAndSettle()
	if !ts.Exists(gluttertest.ByText("bank-password-a")) {
		t.Error("a rebuild for the same item hid the value")
	}
	// Another selection: hidden again.
	ts.PumpWidget(host(kit.LightTheme(), table("two")))
	ts.PumpAndSettle()
	if ts.Exists(gluttertest.ByText("bank-password-a")) || !ts.Exists(gluttertest.ByText(kit.Masked)) {
		t.Error("the value stays revealed on another item")
	}
	if !ts.Exists(gluttertest.ByText("(none)")) {
		t.Error("a field only B has does not say (none) for A")
	}
}

func TestFieldTableSemanticsNeverHoldAProtectedValue(t *testing.T) {
	// gluttertest's semantics finders read RenderObject.DebugSemantics, which only a debug build keeps (Flutter's
	// debugSemantics): in a release build, the default since Glutter 1.17, the first check would pass without looking.
	if !foundation.DebugMode {
		t.Skip("the semantics finders need a debug build of Glutter: go test -tags debug_mode")
	}
	ts := pump(t, kit.LightTheme(), ui.Size{Width: 700, Height: 300}, kit.NewFieldTable(fieldRows(), "A", "B", "k"))
	h := ts.EnsureSemantics()
	defer h.Dispose()
	ts.PumpAndSettle()
	if ts.Exists(gluttertest.BySemanticsLabelMatching(regexpOf("bank-password"))) {
		t.Error("a semantics label holds the password")
	}
	if !ts.Exists(gluttertest.BySemanticsLabelMatching(regexpOf("^Password, differs between A and B"))) {
		t.Error("the row of the password has no label")
	}
}

func TestBannerAndProgressAndDialog(t *testing.T) {
	ts := pump(t, kit.LightTheme(), ui.Size{Width: 500, Height: 400}, widgets.NewColumn([]widgets.Widget{
		kit.NewBanner(kit.ErrorBanner, "The password or key file is wrong", "Check the password and try again"),
		kit.NewProgress("Deriving the key…", foundation.None[float64]()),
	}))
	for _, text := range []string{"The password or key file is wrong", "Check the password and try again", "Deriving the key…"} {
		if !ts.Exists(gluttertest.ByText(text)) {
			t.Errorf("%q is not shown", text)
		}
	}
	var pressed string
	ts = pump(t, kit.LightTheme(), ui.Size{Width: 600, Height: 400}, widgets.NewStack([]widgets.Widget{
		widgets.NewText("behind"),
		kit.NewDialog("Write anyway?", widgets.NewText("KDBX 3.1 cannot store 2 items."), []widgets.Widget{
			kit.NewButton("Cancel", func() { pressed = "cancel" }),
			kit.NewButton("Write", func() { pressed = "write" }, kit.ButtonOpts{Kind: kit.Primary}),
		}, 360),
	}, widgets.StackOpts{Fit: 0}))
	ts.Tap(gluttertest.ByText("Write"))
	if pressed != "write" {
		t.Errorf("pressed %q", pressed)
	}
}

// sample is every widget of the kit once, in a column.
func sample() widgets.Widget {
	c := widgets.NewTextEditingController("/home/me/databases/a.kdbx")
	return widgets.NewColumn([]widgets.Widget{
		kit.NewFileField("Database A", func() {}, kit.FileFieldOpts{Controller: c}),
		gap(kit.SpaceM),
		kit.NewPasswordField("Password", kit.TextFieldOpts{Hint: "Password"}),
		gap(kit.SpaceM),
		kit.NewTextField("Key file", kit.TextFieldOpts{Hint: "Optional key file", Invalid: true}),
		gap(kit.SpaceL),
		kit.NewSegmented("Choice", []kit.Segment{{Label: "Take A", Value: "a"}, {Label: "Take B", Value: "b"}, {Label: "Keep both", Value: "t"}, {Label: "Drop", Value: "d", Disabled: true}}, "b", func(string) {}),
		gap(kit.SpaceL),
		kit.NewListRow(widgets.NewText("differs    Banking / My bank    take B"), "row", true, func(kit.Modifiers) {}),
		kit.NewListRow(widgets.NewText("only-a     Email / Work mail    keep"), "row", false, func(kit.Modifiers) {}),
		gap(kit.SpaceL),
		kit.NewFieldTable(fieldRows(), "A (personal.kdbx)", "B (laptop.kdbx)", "k"),
		gap(kit.SpaceL),
		kit.NewBanner(kit.ErrorBanner, "The password or key file is wrong", "Check the password and try again"),
		gap(kit.SpaceM),
		kit.NewBanner(kit.WarningBanner, "KDBX 3.1 cannot store 2 items", "GroupTags: Root/Email"),
		gap(kit.SpaceM),
		kit.NewBanner(kit.SuccessBanner, "Written and verified", ""),
		gap(kit.SpaceM),
		kit.NewProgress("Deriving the key and writing…", foundation.None[float64]()),
		gap(kit.SpaceL),
		widgets.NewRow([]widgets.Widget{
			kit.NewButton("Primary", func() {}, kit.ButtonOpts{Kind: kit.Primary}), gap2(kit.SpaceM),
			kit.NewButton("Secondary", func() {}), gap2(kit.SpaceM),
			kit.NewButton("Drop", func() {}, kit.ButtonOpts{Kind: kit.Danger}), gap2(kit.SpaceM),
			kit.NewButton("Disabled", nil),
		}),
	}, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch})
}

func gap2(w float64) widgets.Widget {
	return widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: some(w)})
}

func TestGoldens(t *testing.T) {
	for _, th := range themes {
		t.Run(th.name, func(t *testing.T) {
			ts := pump(t, th.theme, ui.Size{Width: 760, Height: 900}, sample())
			ts.MatchesGolden("widgets-" + th.name)
		})
	}
}

// TestProofKit writes the proof image of step 8.1 (go test ./internal/gui/kit -run
// TestProofKit -proof): every widget in both themes, side by side.
func TestProofKit(t *testing.T) {
	ts := testutil.New(t)
	ts.SetViewSize(ui.Size{Width: 1560, Height: 940})
	side := func(th kit.Theme) widgets.Widget {
		return widgets.NewExpanded(kit.NewRoot(th, widgets.NewPadding(painting.NewEdgeInsetsAll(kit.SpaceL), sample())))
	}
	ts.PumpWidget(widgets.NewDirectionality(ui.TextDirectionLTR, widgets.NewRow([]widgets.Widget{side(kit.LightTheme()), side(kit.DarkTheme())})))
	ts.PumpAndSettle()
	testutil.Proof(t, ts, "8.1-kit")
}

func TestDialogGolden(t *testing.T) {
	for _, th := range themes {
		t.Run(th.name, func(t *testing.T) {
			ts := pump(t, th.theme, ui.Size{Width: 640, Height: 360}, widgets.NewStack([]widgets.Widget{
				widgets.NewText("The screen behind the dialog"),
				kit.NewDialog("Write KDBX 3.1 anyway?", widgets.NewText("2 items cannot be stored: the tags of a group, and the time the settings changed."), []widgets.Widget{
					kit.NewButton("Cancel", func() {}),
					kit.NewButton("Write anyway", func() {}, kit.ButtonOpts{Kind: kit.Danger}),
				}, 420),
			}))
			ts.MatchesGolden("dialog-" + th.name)
		})
	}
}
