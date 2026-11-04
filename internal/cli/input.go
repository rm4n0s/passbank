package cli

import (
	"context"
	"crypto/sha256"
	"path/filepath"

	"github.com/rm4n0s/passbank/internal/core/kdbx"
	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

// input is a database that the command opened: the model, how it was stored, the
// hash of its file and the credentials that unlocked it.
type input struct {
	path  string
	db    *model.Database
	info  kdbx.Info
	hash  [32]byte
	creds crypto.Credentials
}

// openInput asks for the credentials of the database at path (by the options, or
// on the terminal) and opens it. The caller wipes the input when it is done.
func openInput(ctx context.Context, e env, opts *credentialOptions, path string) (*input, error) {
	creds, err := opts.load(ctx, e, path)
	if err != nil {
		return nil, err
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
	return &input{path: path, db: db, info: info, hash: sha256.Sum256(data), creds: creds}, nil
}

// wipe overwrites the secrets the input holds.
func (in *input) wipe() {
	in.db.Wipe()
	in.creds.Wipe()
}

// planInput is how the plan file names the input: the hash of its file and the
// name of the file.
func (in *input) planInput() plan.Input {
	return plan.Input{Hash: in.hash, Name: filepath.Base(in.path)}
}
