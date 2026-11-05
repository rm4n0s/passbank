package write

import (
	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/gui/session"
)

// VersionOption is one entry of the version menu.
type VersionOption struct {
	// Label is what the menu shows.
	Label string
	// Value is what the session remembers.
	Value string
	// Version is the version to write; for Auto it is the least that the result
	// needs, which For decides.
	Version kdbx.Version
	// Auto means "KDBX 4": 4.0, or 4.1 when the database holds something that only
	// 4.1 stores (SPEC §5).
	Auto bool
}

// Versions are the versions of the menu. The first is the default.
var Versions = []VersionOption{
	{Label: "KDBX 4", Value: "4", Version: kdbx.Version{Major: 4, Minor: 0}, Auto: true},
	{Label: "4.1", Value: "4.1", Version: kdbx.Version{Major: 4, Minor: 1}},
	{Label: "4.0", Value: "4.0", Version: kdbx.Version{Major: 4, Minor: 0}},
	{Label: "3.1", Value: "3.1", Version: kdbx.Version{Major: 3, Minor: 1}},
	{Label: "3.0", Value: "3.0", Version: kdbx.Version{Major: 3, Minor: 0}},
	{Label: "2.0", Value: "2", Version: kdbx.Version{Major: 2, Minor: 0}},
}

// LookupVersion returns the option with the value; an empty or unknown value is
// the default.
func LookupVersion(value string) VersionOption {
	for _, v := range Versions {
		if v.Value == value {
			return v
		}
	}
	return Versions[0]
}

// For is the version to write the result as.
func (v VersionOption) For(result *model.Database) kdbx.Version {
	if v.Auto && result != nil {
		return kdbx.MinVersion(result)
	}
	return v.Version
}

// The sources of the credentials and of the encryption settings.
const (
	// SourceA is A's.
	SourceA = "a"
	// SourceB is B's.
	SourceB = "b"
	// CredentialsNew is a new password and key file.
	CredentialsNew = "new"
	// EncryptionDefault is the defaults of the version (SPEC §5).
	EncryptionDefault = "default"
)

// CredentialsSource is the source of the credentials, with the default.
func CredentialsSource(o session.Output) string {
	switch o.Credentials {
	case SourceB, CredentialsNew:
		return o.Credentials
	}
	return SourceA
}

// EncryptionSource is the source of the encryption settings, with the default.
func EncryptionSource(o session.Output) string {
	switch o.Encryption {
	case SourceA, SourceB:
		return o.Encryption
	}
	return EncryptionDefault
}

// SettingsFor returns the encryption settings of the new database: A's or B's as
// the file they came from held them (whether the version can hold them is for
// kdbx.Supports to say), or the defaults of the version.
func SettingsFor(source string, a, b *session.Source, version kdbx.Version) kdbx.EncryptionSettings {
	switch source {
	case SourceA:
		return kdbx.SettingsOf(a.Info)
	case SourceB:
		return kdbx.SettingsOf(b.Info)
	}
	return kdbx.DefaultSettings(version)
}
