package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/middleware"
	"github.com/stretchr/testify/assert"
)

func TestRateLimit_BlocksAfterLimit(t *testing.T) {
	handler := middleware.RateLimit(time.Minute, 2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNoContent, w.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}
