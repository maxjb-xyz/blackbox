package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/db"
	"blackbox/server/internal/handlers"
	bbmcp "blackbox/server/internal/mcp"
	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
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
		Enabled:      models.BoolPtr(true),
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
		Enabled:      models.BoolPtr(true),
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthCheck(database, registry)(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "unavailable", resp["oidc"])
}

func TestHealthCheck_DBError_Returns503(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	req := httptest.NewRequest(http.MethodGet, "/api/setup/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthCheck(database, nil)(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "error", resp["database"])
}

func TestPublicHealthCheck_ExposesOnlyLiveness(t *testing.T) {
	database := newTestDB(t)
	registry := auth.NewOIDCRegistry(database)
	require.NoError(t, database.Create(&models.AppSetting{Key: "mcp_auth_token", Value: "mcp-health-secret"}).Error)
	require.NoError(t, database.Create(&models.Node{ID: "node-online", Name: "online", LastSeen: time.Now().UTC(), Status: "online"}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/health", nil)
	w := httptest.NewRecorder()
	handlers.PublicHealthCheck(database, registry)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.NotContains(t, body, "mcp-health-secret")
	assert.NotContains(t, body, "version")
	assert.NotContains(t, body, "commit")
	assert.NotContains(t, body, "nodes")
	assert.NotContains(t, body, "mcp")
	var resp map[string]any
	require.NoError(t, json.NewDecoder(strings.NewReader(body)).Decode(&resp))
	assert.Equal(t, "ok", resp["database"])
	assert.Equal(t, "disabled", resp["oidc"])
	assert.Equal(t, false, resp["oidc_enabled"])
}

func TestHealthCheck_RichLocalSummaryDoesNotExposeSecretsOrProbeProviders(t *testing.T) {
	database := newTestDB(t)
	registry := auth.NewOIDCRegistry(database)
	mcpManager := bbmcp.NewMCPManager(database)
	require.NoError(t, mcpManager.ApplySettings(true, "mcp-health-secret"))

	now := time.Now().UTC()
	require.NoError(t, database.Create(&models.AppSetting{Key: "ai_provider", Value: "openai_compat"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ai_url", Value: "http://127.0.0.1:1"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ai_model", Value: "local-model"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ai_api_key", Value: "ai-health-secret"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "mcp_enabled", Value: "true"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "mcp_auth_token", Value: "mcp-health-secret"}).Error)
	require.NoError(t, database.Create(&models.NotificationDest{
		ID: "notification-1", Name: "Ops", Type: "slack", URL: "https://hooks.example/notification-secret", Enabled: true,
	}).Error)
	require.NoError(t, database.Create(&models.DataSourceInstance{
		ID: ulid.Make().String(), Type: "webhook_uptime_kuma", Scope: models.ScopeServer,
		Name: "Uptime Kuma", Config: `{"secret":"webhook-health-secret"}`, Enabled: true,
	}).Error)
	require.NoError(t, database.Create(&models.DataSourceInstance{
		ID: ulid.Make().String(), Type: "webhook_watchtower", Scope: models.ScopeServer,
		Name: "Watchtower", Config: `{"secret":"disabled-webhook-secret"}`, Enabled: false,
	}).Error)
	require.NoError(t, database.Create(&models.Node{ID: "node-online", Name: "online", LastSeen: now, Status: "online"}).Error)
	require.NoError(t, database.Create(&models.Node{ID: "node-stale", Name: "stale", LastSeen: now.Add(-10 * time.Minute), Status: "online"}).Error)
	require.NoError(t, database.Create(&models.Node{ID: "node-offline", Name: "offline", LastSeen: now, Status: "offline"}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/health", nil)
	w := httptest.NewRecorder()
	handlers.HealthCheckWithBuildInfo(database, registry, mcpManager, "1.2.3", "abc123")(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	for _, secret := range []string{"mcp-health-secret", "ai-health-secret", "webhook-health-secret", "disabled-webhook-secret", "notification-secret"} {
		assert.NotContains(t, body, secret)
	}

	var resp struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		MCP     struct {
			Enabled         bool `json:"enabled"`
			TokenConfigured bool `json:"token_configured"`
			Running         bool `json:"running"`
		} `json:"mcp"`
		AI struct {
			Provider         string `json:"provider"`
			Configured       bool   `json:"configured"`
			Testable         bool   `json:"testable"`
			APIKeyConfigured bool   `json:"api_key_configured"`
		} `json:"ai"`
		Nodes struct {
			Total   int64 `json:"total"`
			Online  int64 `json:"online"`
			Offline int64 `json:"offline"`
			Stale   int64 `json:"stale"`
		} `json:"nodes"`
		Notifications struct {
			Configured bool  `json:"configured"`
			Total      int64 `json:"total"`
			Enabled    int64 `json:"enabled"`
		} `json:"notifications"`
		Webhooks struct {
			Configured bool  `json:"configured"`
			Total      int64 `json:"total"`
			Enabled    int64 `json:"enabled"`
		} `json:"webhooks"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "1.2.3", resp.Version)
	assert.Equal(t, "abc123", resp.Commit)
	assert.True(t, resp.MCP.Enabled)
	assert.True(t, resp.MCP.TokenConfigured)
	assert.True(t, resp.MCP.Running)
	assert.Equal(t, "openai_compat", resp.AI.Provider)
	assert.True(t, resp.AI.Configured)
	assert.True(t, resp.AI.Testable)
	assert.True(t, resp.AI.APIKeyConfigured)
	assert.Equal(t, int64(3), resp.Nodes.Total)
	assert.Equal(t, int64(1), resp.Nodes.Online)
	assert.Equal(t, int64(1), resp.Nodes.Offline)
	assert.Equal(t, int64(1), resp.Nodes.Stale)
	assert.True(t, resp.Notifications.Configured)
	assert.Equal(t, int64(1), resp.Notifications.Total)
	assert.Equal(t, int64(1), resp.Notifications.Enabled)
	assert.True(t, resp.Webhooks.Configured)
	assert.Equal(t, int64(2), resp.Webhooks.Total)
	assert.Equal(t, int64(1), resp.Webhooks.Enabled)
	assert.False(t, strings.Contains(body, "127.0.0.1:1"))
}

func setRegistryProvider(t *testing.T, registry *auth.OIDCRegistry, id string, provider *auth.OIDCProvider) {
	t.Helper()
	registry.SetProvider(id, provider)
}
