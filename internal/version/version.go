// Package version reports which build of Passbank is running. Both programs use
// it for --version.
package version

import (
	"runtime/debug"
	"strings"
)

// String returns the module version and, when the binary was built from a
// checkout, the commit it was built from, such as "v0.1.0" or
// "(devel) 1a2b3c4d5e6f (modified)".
func String() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown)"
	}
	return describe(info)
}

// describe formats the build information of a binary.
func describe(info *debug.BuildInfo) string {
	version := info.Main.Version
	if version == "" {
		version = "(devel)"
	}
	var revision string
	var modified bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	var b strings.Builder
	b.WriteString(version)
	if revision != "" {
		if len(revision) > 12 {
			revision = revision[:12]
		}
		b.WriteString(" " + revision)
		if modified {
			b.WriteString(" (modified)")
		}
	}
	return b.String()
}
