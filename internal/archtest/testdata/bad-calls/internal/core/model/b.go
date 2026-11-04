package model

import f "fmt"

func g() error {

	return f.Errorf("x: %w", nil)
}
