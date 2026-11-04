package model

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
)

var _ = []any{tls.Config{}, net.Dial, http.Get, url.Parse}
