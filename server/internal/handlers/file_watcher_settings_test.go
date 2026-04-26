package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/middleware"
	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentConfig_DefaultsToRedactionEnabled(t *testing.T) {
	database := newTestDB(t)
	config, err := middleware.NewAgentAuthConfig("node-1=secret")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/agent/config", nil)
	req.Header.Set("X-Blackbox-Agent-Key", "secret")
	req.Header.Set("X-Blackbox-Node-Name", "node-1")
	w := httptest.NewRecorder()

	middleware.AgentAuth(config)(handlers.AgentConfig(database)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, true, resp["file_watcher_redact_secrets"])
}

func TestUpdateFileWatcherSettings_PersistsValue(t *testing.T) {
	database := newTestDB(t)

	body := bytes.NewBufferString(`{"redact_secrets":false}`)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/file-watcher", body)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, &auth.Claims{
		UserID:  "admin1",
		IsAdmin: true,
	}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.UpdateFileWatcherSettings(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]bool
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, false, resp["redact_secrets"])

	req2 := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), auth.ClaimsKey, &auth.Claims{
		UserID:  "admin1",
		IsAdmin: true,
	}))
	w2 := httptest.NewRecorder()
	handlers.AdminConfig(database, "secret", nil)(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	var configResp map[string]any
	require.NoError(t, json.NewDecoder(w2.Body).Decode(&configResp))
	assert.Equal(t, false, configResp["file_watcher_redact_secrets"])
}

func TestAgentConfig_ReturnsSystemdUnitsForNode(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.SystemdUnitConfig{
		NodeName: "node-1",
		Units:    `["nginx.service","redis.service"]`,
	}).Error)

	config, err := middleware.NewAgentAuthConfig("node-1=secret")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/agent/config", nil)
	req.Header.Set("X-Blackbox-Agent-Key", "secret")
	req.Header.Set("X-Blackbox-Node-Name", "node-1")
	w := httptest.NewRecorder()
	middleware.AgentAuth(config)(handlers.AgentConfig(database)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	units, ok := resp["systemd_units"].([]interface{})
	require.True(t, ok, "systemd_units should be a list")
	require.Len(t, units, 2)
	require.Equal(t, "nginx.service", units[0])
}

func TestAgentConfig_DisabledSystemdSourceStopsLegacyFallback(t *testing.T) {
	database := newTestDB(t)
	nodeName := "node-1"
	require.NoError(t, database.Create(&models.SystemdUnitConfig{
		NodeName: nodeName,
		Units:    `["nginx.service","redis.service"]`,
	}).Error)
	require.NoError(t, database.Select("*").Create(&models.DataSourceInstance{
		ID: "sys1", Type: "systemd", Scope: "agent", NodeID: &nodeName,
		Name: "Systemd", Config: `{"units":["postgres.service"]}`, Enabled: false,
	}).Error)

	config, err := middleware.NewAgentAuthConfig(nodeName + "=secret")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/agent/config", nil)
	req.Header.Set("X-Blackbox-Agent-Key", "secret")
	req.Header.Set("X-Blackbox-Node-Name", nodeName)
	w := httptest.NewRecorder()
	middleware.AgentAuth(config)(handlers.AgentConfig(database)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	units, ok := resp["systemd_units"].([]interface{})
	require.True(t, ok)
	require.Empty(t, units)
}

func TestAgentConfig_DeletedMigratedSystemdSourceDoesNotReviveLegacyUnits(t *testing.T) {
	database := newTestDB(t)
	nodeName := "node-1"
	require.NoError(t, database.Create(&models.SystemdUnitConfig{
		NodeName: nodeName,
		Units:    `["nginx.service","redis.service"]`,
	}).Error)
	require.NoError(t, handlers.MigrateDataSources(database, ""))

	var inst models.DataSourceInstance
	require.NoError(t, database.Where("type = ? AND node_id = ?", "systemd", nodeName).First(&inst).Error)
	require.NoError(t, database.Delete(&models.DataSourceInstance{}, "id = ?", inst.ID).Error)

	config, err := middleware.NewAgentAuthConfig(nodeName + "=secret")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/agent/config", nil)
	req.Header.Set("X-Blackbox-Agent-Key", "secret")
	req.Header.Set("X-Blackbox-Node-Name", nodeName)
	w := httptest.NewRecorder()
	middleware.AgentAuth(config)(handlers.AgentConfig(database)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	units, ok := resp["systemd_units"].([]interface{})
	require.True(t, ok)
	require.Empty(t, units)
}

func TestAgentConfig_ReturnsFileWatcherEnabledFlag(t *testing.T) {
	database := newTestDB(t)
	nodeName := "node-1"
	require.NoError(t, database.Select("*").Create(&models.DataSourceInstance{
		ID: "fw1", Type: "filewatcher", Scope: "agent", NodeID: &nodeName,
		Name: "File Watcher", Config: `{"redact_secrets":true}`, Enabled: false,
	}).Error)

	config, err := middleware.NewAgentAuthConfig(nodeName + "=secret")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/agent/config", nil)
	req.Header.Set("X-Blackbox-Agent-Key", "secret")
	req.Header.Set("X-Blackbox-Node-Name", nodeName)
	w := httptest.NewRecorder()
	middleware.AgentAuth(config)(handlers.AgentConfig(database)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, false, resp["file_watcher_enabled"])
}

func TestAgentConfig_ReturnsEmptySystemdUnitsWhenNoneConfigured(t *testing.T) {
	database := newTestDB(t)
	config, err := middleware.NewAgentAuthConfig("node-1=secret")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/agent/config", nil)
	req.Header.Set("X-Blackbox-Agent-Key", "secret")
	req.Header.Set("X-Blackbox-Node-Name", "node-1")
	w := httptest.NewRecorder()
	middleware.AgentAuth(config)(handlers.AgentConfig(database)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	units, ok := resp["systemd_units"].([]interface{})
	require.True(t, ok)
	require.Empty(t, units)
}

func TestAgentConfig_RejectsUnauthenticatedRequestsEvenWithNodeHeader(t *testing.T) {
	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agent/config", nil)
	req.Header.Set("X-Blackbox-Node-Name", "node-1")
	w := httptest.NewRecorder()

	handlers.AgentConfig(database)(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}
