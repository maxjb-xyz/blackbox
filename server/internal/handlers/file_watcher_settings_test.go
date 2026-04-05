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
	var resp map[string]bool
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
	handlers.AdminConfig(database, "secret")(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	var configResp map[string]any
	require.NoError(t, json.NewDecoder(w2.Body).Decode(&configResp))
	assert.Equal(t, false, configResp["file_watcher_redact_secrets"])
}
