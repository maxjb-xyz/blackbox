package handlers_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

func TestOIDCProviderLogin_ReturnsServiceUnavailableWhenProviderUnavailable(t *testing.T) {
	database := newTestDB(t)
	registry := auth.NewOIDCRegistry(database)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/provider-1/login", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider_id", "provider-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	handlers.OIDCProviderLogin(database, registry)(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestOIDCProviderCallback_ReturnsServiceUnavailableWhenProviderUnavailable(t *testing.T) {
	database := newTestDB(t)
	registry := auth.NewOIDCRegistry(database)
	database.Create(&models.OIDCState{
		ID:         "01STATEID00000000",
		State:      "test-state",
		Nonce:      "test-nonce",
		ProviderID: "provider-1",
		ExpiresAt:  mustTimeAdd(t, 10),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/provider-1/callback?state=test-state", nil)
	sum := sha256.Sum256([]byte("provider-1"))
	req.AddCookie(&http.Cookie{Name: "oidc_state_" + hex.EncodeToString(sum[:8]), Value: "test-state"})
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider_id", "provider-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	handlers.OIDCProviderCallback(database, registry, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestOIDCProviderCallback_ReturnsBadRequestWhenStateCookieMissing(t *testing.T) {
	database := newTestDB(t)
	registry := auth.NewOIDCRegistry(database)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/provider-1/callback", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider_id", "provider-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	handlers.OIDCProviderCallback(database, registry, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func mustTimeAdd(t *testing.T, minutes int) time.Time {
	t.Helper()
	return time.Now().Add(time.Duration(minutes) * time.Minute)
}

func TestOIDCProviderCallback_LiveProviderFlow(t *testing.T) {
	t.Skip("requires live OIDC provider")
}
