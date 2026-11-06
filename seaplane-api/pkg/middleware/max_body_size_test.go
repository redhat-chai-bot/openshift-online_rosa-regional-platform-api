package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLimitRequestBody_UnderLimit(t *testing.T) {
	called := false
	handler := LimitRequestBody(100)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		body, _ := io.ReadAll(r.Body)
		if string(body) != "hello" {
			t.Errorf("expected body=hello, got %q", string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler was not called for body under limit")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLimitRequestBody_AtLimit(t *testing.T) {
	const limit = 10
	body := bytes.Repeat([]byte("x"), limit)
	called := false

	handler := LimitRequestBody(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler was not called for body at exact limit")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLimitRequestBody_OverLimit_WithContentLength(t *testing.T) {
	// When Content-Length is set and over limit, handler is NOT called.
	const limit = 10
	body := bytes.Repeat([]byte("x"), limit+1)
	called := false

	handler := LimitRequestBody(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	// httptest.NewRequest auto-sets ContentLength from the reader — middleware rejects early.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Error("handler should not be called when Content-Length exceeds limit")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", w.Code)
	}
}

func TestLimitRequestBody_OverLimitResponseCode_ContentLength(t *testing.T) {
	// When Content-Length is declared and exceeds the limit, the middleware
	// returns 413 immediately without calling the handler.
	const limit = 5
	body := bytes.Repeat([]byte("x"), limit+1)
	called := false

	handler := LimitRequestBody(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.ContentLength = int64(len(body)) // declare Content-Length
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Error("handler should not be called when Content-Length exceeds limit")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 for declared oversized Content-Length, got %d", w.Code)
	}
}

func TestLimitRequestBody_OverLimitBodyError_NoContentLength(t *testing.T) {
	// Without Content-Length, the limit is enforced at read time.
	// The handler receives a *http.MaxBytesError when reading the body.
	const limit = 5
	body := bytes.Repeat([]byte("x"), limit+1)
	var gotErr error

	handler := LimitRequestBody(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotErr = io.ReadAll(r.Body)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.ContentLength = -1 // unknown length, no Content-Length header
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if gotErr == nil {
		t.Error("expected *http.MaxBytesError when reading beyond limit, got nil")
	}
	var maxErr *http.MaxBytesError
	if !errors.As(gotErr, &maxErr) {
		t.Errorf("expected *http.MaxBytesError, got %T: %v", gotErr, gotErr)
	}
}

func TestLimitRequestBody_ZeroUsesDefault(t *testing.T) {
	// maxBytes=0 should fall back to DefaultMaxBodySize (not zero, which would reject all)
	body := bytes.Repeat([]byte("x"), 1024) // 1 KB — well under default 1 MiB
	called := false

	handler := LimitRequestBody(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler not called — zero limit incorrectly rejected a small body")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLimitRequestBody_NegativeUsesDefault(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 1024)
	called := false

	handler := LimitRequestBody(-1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler not called — negative limit incorrectly rejected a small body")
	}
}

func TestLimitRequestBody_HandlerNotCalledOnEmptyBodyOverLimit(t *testing.T) {
	// GET requests with no body should always pass regardless of limit
	called := false
	handler := LimitRequestBody(1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("GET with no body should always reach handler")
	}
}
