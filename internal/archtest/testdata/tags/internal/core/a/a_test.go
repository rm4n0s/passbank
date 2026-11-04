package a

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

func assertRoute(t *testing.T, err error, route string) {
	t.Helper()
	e, _ := errors.FromError(err)
	if !e.HasRoute(route) {
		t.Error("route")
	}
}

func TestTable(t *testing.T) {
	assertRoute(t, Read(), "Read.ReadFailed")
}

func TestSave(t *testing.T) {
	e, _ := errors.FromError((&T{}).Save())
	if e.Tag != "SaveFailed" {
		t.Error("tag")
	}
}

func TestSentinel(t *testing.T) {
	if !errors.Is(Sentinel(), &errors.Error{Tag: "SentinelTag"}) {
		t.Error("sentinel")
	}
}

func TestFake(t *testing.T) {
	fake := errors.New("FakeOnly", "a fake")
	_ = fake
}
