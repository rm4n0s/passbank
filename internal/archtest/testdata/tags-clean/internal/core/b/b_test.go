package b

import (
	"testing"

	"github.com/rm4n0s/errors"
)

func TestOpen(t *testing.T) {
	e, _ := errors.FromError(Open())
	if !e.HasRoute("Open.OpenFailed") {
		t.Error("route")
	}
}
