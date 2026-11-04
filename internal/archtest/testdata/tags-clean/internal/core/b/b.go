package b

import "github.com/rm4n0s/errors"

func Open() error { return errors.New("OpenFailed", "open failed") }
