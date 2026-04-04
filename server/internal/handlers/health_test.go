package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"unsafe"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCheck_DatabaseOKOIDCDisabled(t *testing.T) {
	database := newTestDB(t)
	registry := auth.NewOIDCRegistry(database)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthCheck(database, registry)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "ok", resp["database"])
	assert.Equal(t, "disabled", resp["oidc"])
	assert.Equal(t, false, resp["oidc_enabled"])
}

func TestHealthCheck_OIDCReady(t *testing.T) {
	database := newTestDB(t)
	registry := auth.NewOIDCRegistry(database)
	require.NoError(t, database.Create(&models.OIDCProviderConfig{
		ID:           "provider-1",
		Name:         "SSO",
		Issuer:       "https://issuer.example.com",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://app.example.com/callback",
		Enabled:      true,
	}).Error)
	setRegistryProvider(t, registry, "provider-1", &auth.OIDCProvider{})

	req := httptest.NewRequest(http.MethodGet, "/api/setup/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthCheck(database, registry)(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "ok", resp["oidc"])
	assert.Equal(t, true, resp["oidc_enabled"])
}

func TestHealthCheck_OIDCEnabledButNotReady(t *testing.T) {
	database := newTestDB(t)
	registry := auth.NewOIDCRegistry(database)
	require.NoError(t, database.Create(&models.OIDCProviderConfig{
		ID:           "provider-1",
		Name:         "SSO",
		Issuer:       "https://issuer.example.com",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://app.example.com/callback",
		Enabled:      true,
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthCheck(database, registry)(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "unavailable", resp["oidc"])
}

func setRegistryProvider(t *testing.T, registry *auth.OIDCRegistry, id string, provider *auth.OIDCProvider) {
	t.Helper()

	field := reflect.ValueOf(registry).Elem().FieldByName("providers")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(
		reflect.ValueOf(map[string]*auth.OIDCProvider{id: provider}),
	)
}
