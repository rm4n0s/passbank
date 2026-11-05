// Package session holds the state that the three screens of the GUI share: the two
// databases that are unlocked, the plan with the user's choices, and the settings
// of the file to write. It is the only place where the secrets of an open session
// live, and Wipe overwrites them (SPEC §13: leaving the Open screen's flow, or
// closing the window, drops every unlocked database).
package session

import (
	"context"
	"crypto/sha256"
	"path/filepath"

	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/plan"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// Source is one input database, unlocked.
type Source struct {
	// Path is the file that was opened.
	Path string
	// DB is the database; it holds secrets.
	DB *model.Database
	// Info is how the file is stored, and what it holds in numbers.
	Info kdbx.Info
	// Hash is the SHA-256 of the file's bytes, for the plan file.
	Hash [32]byte
	// Creds are the credentials that unlocked it; the output can reuse them.
	Creds crypto.Credentials
}

// PlanInput is how a plan file names the source.
func (s *Source) PlanInput() plan.Input {
	return plan.Input{Hash: s.Hash, Name: filepath.Base(s.Path)}
}

// Wipe overwrites the secrets the source holds. It must not be used afterwards.
func (s *Source) Wipe() {
	if s == nil {
		return
	}
	s.DB.Wipe()
	s.Creds.Wipe()
}

// Unlock reads the file at path and decrypts it with the password and the key file
// (either may be empty; both empty is NoCredentials). It takes ownership of
// password and wipes it. It does the slow work, so a screen calls it off the UI
// thread, with a ctx that its Cancel button ends.
func Unlock(ctx context.Context, path string, password secret.Bytes, keyFilePath string) (*Source, error) {
	var creds crypto.Credentials
	if keyFilePath != "" {
		key, err := crypto.ReadKeyFile(ctx, keyFilePath)
		if err != nil {
			password.Wipe()
			return nil, err
		}
		creds.HasKeyFile, creds.KeyFile = true, key
	}
	if password.Len() > 0 {
		creds.HasPassword, creds.Password = true, password
	} else {
		password.Wipe()
	}
	data, err := kdbx.ReadFile(ctx, path)
	if err != nil {
		creds.Wipe()
		return nil, err
	}
	db, info, err := kdbx.OpenBytes(ctx, data, creds)
	if err != nil {
		creds.Wipe()
		return nil, err
	}
	return &Source{Path: path, DB: db, Info: info, Hash: sha256.Sum256(data), Creds: creds}, nil
}

// Session is the state of one run of the application.
type Session struct {
	// A and B are the unlocked inputs; nil while a side is locked.
	A, B *Source
	// Plan is the user's choices; nil until the Review screen starts.
	Plan *plan.Plan
	// Decided[i] says that the user has chosen for Plan.Changes[i] (or loaded a plan
	// that does); the others still hold their default. The Review screen's
	// "undecided" filter lists those.
	Decided []bool
}

// New returns an empty session.
func New() *Session { return &Session{} }

// Ready says whether both inputs are unlocked.
func (s *Session) Ready() bool { return s.A != nil && s.B != nil }

// Swap exchanges A and B. The plan was made for the old order, so it is dropped.
func (s *Session) Swap() {
	s.A, s.B = s.B, s.A
	s.dropPlan()
}

// SetA unlocks side A with a source, wiping the one it replaces; the plan is
// dropped, because it fitted the old one.
func (s *Session) SetA(src *Source) {
	s.A.Wipe()
	s.A = src
	s.dropPlan()
}

// SetB is SetA for side B.
func (s *Session) SetB(src *Source) {
	s.B.Wipe()
	s.B = src
	s.dropPlan()
}

// Wipe overwrites every secret of the session and empties it.
func (s *Session) Wipe() {
	s.A.Wipe()
	s.B.Wipe()
	s.A, s.B = nil, nil
	s.dropPlan()
}

func (s *Session) dropPlan() { s.Plan, s.Decided = nil, nil }
