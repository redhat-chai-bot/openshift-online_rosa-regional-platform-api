package handlers

// Integration tests verifying that the LimitRequestBody middleware correctly
// rejects oversized request bodies before handlers read them, using the DNS
// reservation handler as a representative endpoint (smallest setup overhead).

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openshift-online/rosa-seaplane-api/seaplane-api/pkg/middleware"
)

// withBodyLimit wraps a handler with LimitRequestBody at the given byte limit.
func withBodyLimit(limit int64, h http.Handler) http.Handler {
	return middleware.LimitRequestBody(limit)(h)
}

func TestHandlerRejectsOversizedBody(t *testing.T) {
	const limit = 100
	handler, _ := newDNSReservationTestHandler(t)

	// Body is larger than the configured limit; set Content-Length so the
	// middleware can reject before the handler reads the body.
	body := bytes.Repeat([]byte("x"), limit+1)
	req := httptest.NewRequest(http.MethodPost, "/api/v0/dns_reservations", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req = req.WithContext(testContext(dnsReservationTestAccountID))
	w := httptest.NewRecorder()

	withBodyLimit(limit, http.HandlerFunc(handler.Create)).ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 for oversized body, got %d", w.Code)
	}
}

func TestHandlerAcceptsBodyUnderLimit(t *testing.T) {
	const limit = 512
	handler, _ := newDNSReservationTestHandler(t)

	// Valid JSON body well under the limit
	body := `{"metadata":{"name":"zone-a"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v0/dns_reservations", bytes.NewBufferString(body))
	req = req.WithContext(testContext(dnsReservationTestAccountID))
	w := httptest.NewRecorder()

	withBodyLimit(limit, http.HandlerFunc(handler.Create)).ServeHTTP(w, req)

	// 201 or 409 (already exists) are both success — neither is 413
	if w.Code == http.StatusRequestEntityTooLarge {
		t.Errorf("valid body under limit was incorrectly rejected with 413")
	}
}

func TestHandlerDefaultLimitAcceptsTypicalClusterPayload(t *testing.T) {
	// Build a body >1 MiB but <10 MiB (DefaultMaxBodySize) so the test catches
	// a regression to the old upstream hardcoded 1 MiB limit.
	const pad = (1 << 20) + 1 // 1 MiB + 1 byte — over the old limit, under DefaultMaxBodySize (10 MiB)

	var body bytes.Buffer
	body.WriteString(`{"metadata":{"name":"my-cluster","annotations":{"pad":"`)
	body.Write(bytes.Repeat([]byte("x"), pad))
	body.WriteString(`"}}}`)

	if int64(body.Len()) >= middleware.DefaultMaxBodySize {
		t.Fatalf("test body (%d bytes) exceeds DefaultMaxBodySize (%d) — update pad constant", body.Len(), middleware.DefaultMaxBodySize)
	}

	handler, _ := newDNSReservationTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v0/dns_reservations", &body)
	req = req.WithContext(testContext(dnsReservationTestAccountID))
	w := httptest.NewRecorder()

	withBodyLimit(middleware.DefaultMaxBodySize, http.HandlerFunc(handler.Create)).ServeHTTP(w, req)

	if w.Code == http.StatusRequestEntityTooLarge {
		t.Error("body over 1 MiB but under DefaultMaxBodySize (10 MiB) was incorrectly rejected")
	}
}
