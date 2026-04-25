package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerTokenMiddleware(t *testing.T) {
	nextCalled := false
	handler := BearerTokenMiddleware("secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}
}

func TestBearerTokenMiddlewareUnauthorized(t *testing.T) {
	handler := BearerTokenMiddleware("secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	for _, authHeader := range []string{"", "secret", "Bearer wrong"} {
		req := httptest.NewRequest(http.MethodGet, "/sse", nil)
		req.Header.Set("Authorization", authHeader)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("auth header %q: expected 401, got %d", authHeader, rr.Code)
		}
	}
}
