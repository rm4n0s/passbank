// Package fonts embeds the fonts of the GUI: the Go fonts (BSD-3-Clause, from
// golang.org/x/image/font/gofont). The app and the tests use only these, never
// the fonts of the system, so a window looks the same on every machine (SPEC §13).
package fonts

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/rm4n0s/glutter/widgets"
)

// The family names that TextStyle.FontFamily uses.
const (
	// Sans is the family of the interface text.
	Sans = "Go"
	// Mono is the monospace family, for UUIDs and other text where every
	// character must line up.
	Mono = "Go Mono"
)

// Face is one font file: a family name and the content of a TrueType file.
type Face struct {
	// Family is the family the face belongs to; faces of one family differ in
	// weight and slant.
	Family string
	// Data is the TrueType file.
	Data []byte
}

// Faces returns every font file, regular, bold, italic and bold italic of each
// family. Tests turn them into gluttertest fonts (internal/gui/testutil).
func Faces() []Face {
	return []Face{
		{Sans, goregular.TTF}, {Sans, gobold.TTF}, {Sans, goitalic.TTF}, {Sans, gobolditalic.TTF},
		{Mono, gomono.TTF}, {Mono, gomonobold.TTF}, {Mono, gomonoitalic.TTF}, {Mono, gomonobolditalic.TTF},
	}
}

// Sources returns Faces as the list that RunAppOpts.Fonts takes.
func Sources() []widgets.FontSource {
	faces := Faces()
	out := make([]widgets.FontSource, len(faces))
	for i, f := range faces {
		out[i] = widgets.FontSource{Family: f.Family, Data: f.Data}
	}
	return out
}
