package fonts

import (
	"testing"

	"github.com/rm4n0s/glutter/foundation"
	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/painting"
	"github.com/rm4n0s/glutter/ui"
	"github.com/rm4n0s/glutter/widgets"
)

func TestFaces(t *testing.T) {
	counts := map[string]int{}
	for _, f := range Faces() {
		if len(f.Data) < 10_000 {
			t.Errorf("%s: font data is only %d bytes", f.Family, len(f.Data))
		}
		counts[f.Family]++
	}
	// Regular, bold, italic and bold italic of each family.
	if counts[Sans] != 4 || counts[Mono] != 4 || len(counts) != 2 {
		t.Errorf("faces per family = %v, want 4 each of %q and %q", counts, Sans, Mono)
	}
	sources := Sources()
	if len(sources) != len(Faces()) {
		t.Fatalf("Sources has %d entries, Faces %d", len(sources), len(Faces()))
	}
	for i, s := range sources {
		if s.Family != Faces()[i].Family || len(s.Data) == 0 || s.Asset != "" {
			t.Errorf("source %d = {%q, %d bytes, asset %q}", i, s.Family, len(s.Data), s.Asset)
		}
	}
}

// widthOf lays the text out in the family and returns its width.
func widthOf(t *testing.T, family, text string) float64 {
	t.Helper()
	var faces []gluttertest.Font
	for _, f := range Faces() {
		faces = append(faces, gluttertest.Font{Family: f.Family, Data: f.Data})
	}
	ts := gluttertest.New(t, gluttertest.Opts{Fonts: faces})
	style := painting.NewTextStyle(painting.TextStyleOpts{FontFamily: family, FontSize: foundation.Some(20.0)})
	ts.PumpWidget(widgets.NewDirectionality(ui.TextDirectionLTR,
		widgets.NewAlign(widgets.NewText(text, widgets.TextOpts{Style: style}),
			widgets.AlignOpts{Alignment: painting.AlignmentTopLeft})))
	return ts.Size(gluttertest.ByType[*widgets.Text]()).Width
}

// TestFamiliesLoad checks that both families really are loaded: a monospace font
// gives "iiii" and "WWWW" one width, a proportional font does not.
func TestFamiliesLoad(t *testing.T) {
	tests := []struct {
		family string
		mono   bool
	}{{Mono, true}, {Sans, false}}
	for _, tt := range tests {
		narrow, wide := widthOf(t, tt.family, "iiii"), widthOf(t, tt.family, "WWWW")
		if tt.mono && narrow != wide {
			t.Errorf("%s: iiii is %v wide and WWWW %v; a monospace font gives one width", tt.family, narrow, wide)
		}
		if !tt.mono && narrow >= wide {
			t.Errorf("%s: iiii is %v wide and WWWW %v; a proportional font gives iiii less", tt.family, narrow, wide)
		}
	}
}
