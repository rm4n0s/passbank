package a

import "github.com/rm4n0s/errors"

func Open() error { return errors.New("OpenFailed", "open failed") }

func Read() error { return errors.NewErr("ReadFailed", nil) }

type T struct{}

func (t *T) Save() error { return errors.NewErrf("SaveFailed", "save %d", 1) }

func Sentinel() error { return errors.New("SentinelTag", "sentinel") }

func Untested() error { return errors.New("NeverTested", "never") }

func Fake() error { return errors.New("FakeOnly", "only a test fake has this tag") }

func Listed() error { return errors.New("ListedOnly", "listed in the exceptions file") }

// CommentOnly is mentioned only in a comment: "CommentOnly".
func CommentOnly() error { return errors.New("CommentOnly", "x") }

func Dynamic(tag string) error { return errors.New(tag, "dynamic") }
