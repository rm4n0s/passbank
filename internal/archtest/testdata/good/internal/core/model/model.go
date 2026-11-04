package model

import (
	"fmt"

	"github.com/rm4n0s/errors"
)

var X = fmt.Sprint("x")

func f() error { return errors.New("Tag", "message") }
