package model

import "fmt"

func f() error {
	return fmt.Errorf("x")
}

var _ = fmt.Sprintf("fine")
