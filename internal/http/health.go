package http

import (
	"io"
	nethttp "net/http"
)

// HealthHandler returns a simple health check endpoint.
func HealthHandler() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(nethttp.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})
}
