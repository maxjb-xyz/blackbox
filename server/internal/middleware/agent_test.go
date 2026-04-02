package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/middleware"
	"github.com/stretchr/testify/assert"
)

func TestAgentAuth_ValidToken(t *testing.T) {
	reached := false
	handler := middleware.AgentAuth("secret-token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	assert.True(t, reached)
}

func TestAgentAuth_WrongToken(t *testing.T) {
	handler := middleware.AgentAuth("secret-token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAgentAuth_MissingToken(t *testing.T) {
	handler := middleware.AgentAuth("secret-token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
