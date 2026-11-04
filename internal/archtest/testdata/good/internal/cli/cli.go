package cli

import (
	"net/url"

	"golang.org/x/term"

	"example.com/passbank/internal/core/model"
)

func Main() {
	_, _ = url.Parse(model.X)
	_ = term.IsTerminal(0)
}
