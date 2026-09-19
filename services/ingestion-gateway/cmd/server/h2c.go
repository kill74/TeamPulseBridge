package main

import (
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// newH2CHandler wraps the mux with HTTP/2 cleartext for internal mesh use.
// Edge stays HTTP/1 by default; opt in via HTTP_H2C_ENABLED=true.
func newH2CHandler(handler http.Handler) http.Handler {
	srv := &http2.Server{
		MaxConcurrentStreams: 250,
	}
	return h2c.NewHandler(handler, srv)
}
