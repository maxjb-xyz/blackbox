package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/handlers"
	"github.com/stretchr/testify/assert"
)

func TestOIDCLogin_ReturnsServiceUnavailableWhenProviderNil(t *testing.T) {
	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/login", nil)
	w := httptest.NewRecorder()

	handlers.OIDCLogin(database, nil)(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestOIDCCallback_ReturnsServiceUnavailableWhenProviderNil(t *testing.T) {
	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback", nil)
	w := httptest.NewRecorder()

	handlers.OIDCCallback(database, nil, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestOIDCCallback_ReturnsBadRequestWhenStateCookieMissing(t *testing.T) {
	t.Skip("requires live OIDC provider")
}
