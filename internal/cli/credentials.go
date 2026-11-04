package cli

import (
	"bytes"
	"context"
	"flag"
	"io"
	"os"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// maxPasswordFile is the largest password file Passbank reads: the password is its
// first line, so a larger file is a mistake.
const maxPasswordFile = 64 << 10

// credentialOptions are the options that name the credentials of one database:
// --keyfile, --password-file and --no-password, with a suffix for the input they
// belong to (SPEC §12). A password is never an option or an environment variable.
type credentialOptions struct {
	keyFile      string
	passwordFile string
	noPassword   bool
	// suffix is "" for a command with one database, and "-a" or "-b" for the inputs
	// of the others; it is only used in messages.
	suffix string
}

// addCredentialFlags registers the credential options for a database on fs.
func addCredentialFlags(fs *flag.FlagSet, suffix string) *credentialOptions {
	o := &credentialOptions{suffix: suffix}
	fs.StringVar(&o.keyFile, "keyfile"+suffix, "", "key file of the database")
	fs.StringVar(&o.passwordFile, "password-file"+suffix, "", "file whose first line is the password (readable only by its owner)")
	fs.BoolVar(&o.noPassword, "no-password"+suffix, false, "the database has no password")
	return o
}

// check finds options that contradict each other.
func (o *credentialOptions) check() error {
	if o.noPassword && o.passwordFile != "" {
		return errors.New("UsageInvalid", "--no-password"+o.suffix+" and --password-file"+o.suffix+" contradict each other")
	}
	return nil
}

// load makes the credentials from the options. A password that no option gives is
// read from the terminal without echo, with the prompt naming the database. An
// empty password is no password component, as in KeePassXC; --no-password says so
// without asking.
func (o *credentialOptions) load(ctx context.Context, e env, name string) (crypto.Credentials, error) {
	var c crypto.Credentials
	if o.keyFile != "" {
		key, err := crypto.ReadKeyFile(ctx, o.keyFile)
		if err != nil {
			return crypto.Credentials{}, err
		}
		c.HasKeyFile, c.KeyFile = true, key
	}
	if o.noPassword {
		return c, nil
	}
	var password secret.Bytes
	var err error
	if o.passwordFile != "" {
		password, err = readPasswordFile(ctx, o.passwordFile)
	} else {
		if e.term == nil || !e.term.IsTerminal() {
			c.Wipe()
			return crypto.Credentials{}, errors.New("PasswordNeedsTerminal", "there is no terminal to ask for the password; use --password-file"+o.suffix+" or --no-password"+o.suffix, "database", name)
		}
		password, err = e.term.ReadPassword("Password for " + name + ": ")
	}
	if err != nil {
		c.Wipe()
		return crypto.Credentials{}, err
	}
	if password.Len() == 0 {
		return c, nil
	}
	c.HasPassword, c.Password = true, password
	return c, nil
}

// readPasswordFile reads the first line of a file as a password. The file must be
// a regular file that group and others cannot read, which is checked on the open
// file, so that it cannot change between the check and the read.
func readPasswordFile(ctx context.Context, path string) (secret.Bytes, error) {
	if err := ctx.Err(); err != nil {
		return secret.Bytes{}, errors.NewErr("ReadCancelled", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return secret.Bytes{}, errors.NewErr("PasswordFileReadFailed", err).SetMetadata("path", path)
	}
	defer func() { _ = f.Close() }() // the file was only read
	fi, err := f.Stat()
	if err != nil {
		return secret.Bytes{}, errors.NewErr("PasswordFileReadFailed", err).SetMetadata("path", path)
	}
	if !fi.Mode().IsRegular() {
		return secret.Bytes{}, errors.New("PasswordFileReadFailed", "the password file is not a regular file", "path", path)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return secret.Bytes{}, errors.New("PasswordFileTooOpen", "the password file can be read by others; make it readable only by you (chmod 600)", "path", path, "mode", perm.String())
	}
	data, err := io.ReadAll(io.LimitReader(f, maxPasswordFile+1))
	if err != nil {
		return secret.Bytes{}, errors.NewErr("PasswordFileReadFailed", err).SetMetadata("path", path)
	}
	defer clear(data)
	if len(data) > maxPasswordFile {
		return secret.Bytes{}, errors.New("PasswordFileReadFailed", "the password file is too large", "path", path, "limit", maxPasswordFile)
	}
	line := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		line = data[:i]
	}
	line = bytes.TrimSuffix(line, []byte{'\r'})
	return secret.New(append([]byte(nil), line...)), nil
}

// parseInterleaved parses flags that may come before and after the positional
// arguments, which the flag package alone does not allow. fs must not exit.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, errors.New("UsageInvalid", err.Error())
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}
