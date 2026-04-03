package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/middleware"
	"github.com/stretchr/testify/assert"
)

func TestJWTAuth_ValidToken(t *testing.T) {
	secret := "test-secret"
	token, _ := auth.IssueJWT("user-1", "alice", false, secret, time.Hour)

	reached := false
	handler := middleware.JWTAuth(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		claims, ok := auth.ClaimsFromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, "user-1", claims.UserID)
		assert.Equal(t, "alice", claims.Username)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(httptest.NewRecorder(), req)
	assert.True(t, reached)
}

func TestJWTAuth_MissingToken(t *testing.T) {
	handler := middleware.JWTAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	handler := middleware.JWTAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not.a.token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
