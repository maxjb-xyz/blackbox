package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCheck_DatabaseOKOIDCDisabled(t *testing.T) {
	database := newTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthCheck(database, false, false)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "ok", resp["database"])
	assert.Equal(t, "disabled", resp["oidc"])
	assert.Equal(t, false, resp["oidc_enabled"])
}

func TestHealthCheck_OIDCReady(t *testing.T) {
	database := newTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthCheck(database, true, true)(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "ok", resp["oidc"])
	assert.Equal(t, true, resp["oidc_enabled"])
}

func TestHealthCheck_OIDCEnabledButNotReady(t *testing.T) {
	database := newTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthCheck(database, true, false)(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "unavailable", resp["oidc"])
}
