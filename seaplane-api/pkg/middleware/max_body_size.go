package middleware

import (
	"log/slog"
	"net/http"

	"github.com/openshift-online/rosa-seaplane-api/seaplane-api/pkg/api"
)

// DefaultMaxBodySize is the default maximum request body size (10 MiB).
// This matches the AWS API Gateway maximum payload size so that legitimate
// requests reaching the Platform API are never rejected by this limit alone.
// Endpoints with tighter requirements should apply a route-specific limit.
const DefaultMaxBodySize int64 = 10 << 20 // 10 MiB

// LimitRequestBody returns middleware that enforces a maximum request body size.
//
// Two defenses are applied:
//  1. If the request declares a Content-Length exceeding maxBytes, the request
//     is rejected immediately with 413 before any body is read.
//  2. r.Body is wrapped with http.MaxBytesReader so that handlers reading beyond
//     maxBytes receive a *http.MaxBytesError instead of buffering the full payload.
//
// If maxBytes is 0 or negative, DefaultMaxBodySize is used.
func LimitRequestBody(maxBytes int64) func(http.Handler) http.Handler {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBodySize
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Reject immediately when Content-Length is declared and over limit.
			if r.ContentLength > maxBytes {
				if err := api.WriteError(w, api.ErrRequestBodyTooLarge); err != nil {
					slog.Default().Error("failed to write 413 response", "error", err)
				}
				return
			}
			// Wrap body so reads beyond maxBytes return *http.MaxBytesError.
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
