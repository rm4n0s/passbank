package kit

import (
	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/rendering"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"
)

// BannerKind is what a Banner says.
type BannerKind int

const (
	// ErrorBanner reports something that failed.
	ErrorBanner BannerKind = iota
	// WarningBanner reports something that needs the user's attention.
	WarningBanner
	// SuccessBanner reports something that went well.
	SuccessBanner
)

// Banner is a message in a colored box. Errors and warnings are announced to a
// screen reader as live regions.
type Banner struct {
	widgets.StatelessWidget
	kind    BannerKind
	message string
	detail  string
}

// NewBanner returns a banner with a message and, below it in smaller text, an
// optional detail.
func NewBanner(kind BannerKind, message, detail string, opts ...widgets.WidgetOpts) *Banner {
	return &Banner{StatelessWidget: widgets.NewStatelessWidgetBase(opts...), kind: kind, message: message, detail: detail}
}

// Message is the text of the banner.
func (b *Banner) Message() string { return b.message }

// Build builds the box.
func (b *Banner) Build(bc widgets.BuildContext) widgets.Widget {
	th := ThemeOf(bc)
	bg, fg, tag := th.DangerSurface, th.Danger, "Error"
	switch b.kind {
	case WarningBanner:
		bg, fg, tag = th.WarningSurface, th.Warning, "Warning"
	case SuccessBanner:
		bg, fg, tag = th.SuccessSurface, th.Success, "Done"
	}
	children := []widgets.Widget{widgets.NewText(b.message, widgets.TextOpts{Style: TextStyle(SizeBody, ui.FontWeightW700, fg)})}
	if b.detail != "" {
		children = append(children, widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(SpaceS)}),
			widgets.NewText(b.detail, widgets.TextOpts{Style: TextStyle(SizeSmall, ui.FontWeightW400, th.Text)}))
	}
	box := widgets.NewContainer(widgets.NewColumn(children, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStart}), widgets.ContainerOpts{
		Padding: painting.NewEdgeInsetsAll(SpaceM),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color:        foundation.Some(bg),
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(fg)}),
			BorderRadius: painting.NewBorderRadiusCircular(Radius),
		}),
	})
	return widgets.NewSemantics(box, widgets.SemanticsOpts{
		Container: true, LiveRegion: foundation.Some(b.kind != SuccessBanner),
		Label: foundation.Some(tag + ": " + b.message + detailSuffix(b.detail)),
	})
}

func detailSuffix(detail string) string {
	if detail == "" {
		return ""
	}
	return ". " + detail
}

// Progress is a bar and a caption that say something is being done. It has no
// animation: it shows that the work has started and how far it is, when that is known.
type Progress struct {
	widgets.StatelessWidget
	caption string
	value   foundation.Opt[float64]
}

// NewProgress returns the bar; value is the fraction done, from 0 to 1, or none when
// it is not known (the bar is then drawn partly filled, and the caption is all that
// says what is going on).
func NewProgress(caption string, value foundation.Opt[float64], opts ...widgets.WidgetOpts) *Progress {
	return &Progress{StatelessWidget: widgets.NewStatelessWidgetBase(opts...), caption: caption, value: value}
}

// Caption is the text above the bar.
func (p *Progress) Caption() string { return p.caption }

// Build builds the bar.
func (p *Progress) Build(bc widgets.BuildContext) widgets.Widget {
	th := ThemeOf(bc)
	fraction := p.value.OrElse(0.3)
	fraction = max(0, min(1, fraction))
	bar := widgets.NewContainer(widgets.NewFractionallySizedBox(widgets.NewContainer(nil, widgets.ContainerOpts{
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{Color: foundation.Some(th.Primary), BorderRadius: painting.NewBorderRadiusCircular(3)}),
	}), widgets.FractionallySizedBoxOpts{WidthFactor: foundation.Some(fraction), Alignment: painting.AlignmentCenterLeft}), widgets.ContainerOpts{
		Height: foundation.Some(6.0),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color: foundation.Some(th.Raised), BorderRadius: painting.NewBorderRadiusCircular(3),
		}),
	})
	return widgets.NewSemantics(widgets.NewColumn([]widgets.Widget{
		widgets.NewText(p.caption, widgets.TextOpts{Style: th.Body()}),
		widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(SpaceS)}),
		bar,
	}, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch}),
		widgets.SemanticsOpts{Container: true, Label: foundation.Some(p.caption), LiveRegion: foundation.Some(true)})
}

// Dialog is a modal box over the screen: a dimmed barrier that blocks the screen
// below, and a card with a title, a body and a row of actions. It is a widget to
// put in a Stack above the screen; the screen shows it while it is open and closes
// it from the action callbacks.
type Dialog struct {
	widgets.StatelessWidget
	title   string
	body    widgets.Widget
	actions []widgets.Widget
	width   float64
}

// NewDialog returns the dialog; width is the width of the card.
func NewDialog(title string, body widgets.Widget, actions []widgets.Widget, width float64, opts ...widgets.WidgetOpts) *Dialog {
	return &Dialog{StatelessWidget: widgets.NewStatelessWidgetBase(opts...), title: title, body: body, actions: actions, width: width}
}

// Title is the title of the dialog.
func (d *Dialog) Title() string { return d.title }

// Build builds the barrier and the card.
func (d *Dialog) Build(bc widgets.BuildContext) widgets.Widget {
	th := ThemeOf(bc)
	card := widgets.NewContainer(widgets.NewColumn([]widgets.Widget{
		widgets.NewSemantics(widgets.NewText(d.title, widgets.TextOpts{Style: TextStyle(18, ui.FontWeightW700, th.Text)}),
			widgets.SemanticsOpts{Header: foundation.Some(true)}),
		widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(SpaceM)}),
		widgets.NewDefaultTextStyle(th.Body(), d.body),
		widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Height: foundation.Some(SpaceL)}),
		widgets.NewRow(spaced(d.actions), widgets.RowOpts{MainAxisAlignment: rendering.MainAxisAlignmentEnd}),
	}, widgets.ColumnOpts{CrossAxisAlignment: rendering.CrossAxisAlignmentStretch, MainAxisSize: rendering.MainAxisSizeMin}), widgets.ContainerOpts{
		Width:   foundation.Some(d.width),
		Padding: painting.NewEdgeInsetsAll(SpaceL),
		Decoration: painting.NewBoxDecoration(painting.BoxDecorationOpts{
			Color:        foundation.Some(th.Surface),
			Border:       painting.NewBorderAll(painting.BorderSideOpts{Color: foundation.Some(th.Border)}),
			BorderRadius: painting.NewBorderRadiusCircular(Radius * 2),
		}),
	})
	barrier := widgets.NewGestureDetector(widgets.NewColoredBox(ui.NewColor(0x99000000), widgets.NewSizedBoxExpand(nil)), widgets.GestureDetectorOpts{
		Behavior: rendering.HitTestBehaviorOpaque,
	})
	return widgets.NewSemantics(widgets.NewStack([]widgets.Widget{barrier, widgets.NewCenter(card)}, widgets.StackOpts{Fit: rendering.StackFitExpand}),
		widgets.SemanticsOpts{Container: true, ExplicitChildNodes: true, ScopesRoute: foundation.Some(true), NamesRoute: foundation.Some(true), Label: foundation.Some(d.title)})
}

// spaced puts a gap between widgets.
func spaced(items []widgets.Widget) []widgets.Widget {
	out := make([]widgets.Widget, 0, 2*len(items))
	for i, it := range items {
		if i > 0 {
			out = append(out, widgets.NewSizedBox(nil, widgets.SizedBoxOpts{Width: foundation.Some(SpaceM)}))
		}
		out = append(out, it)
	}
	return out
}
