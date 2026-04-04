package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/db"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/middleware"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := db.Init(":memory:")
	require.NoError(t, err)
	return database
}

func authenticatedAgentRequest(t *testing.T, entry types.Entry, nodeName string) (*http.Request, *httptest.ResponseRecorder, func(http.Handler) http.Handler) {
	t.Helper()

	body, err := json.Marshal(entry)
	require.NoError(t, err)

	config, err := middleware.NewAgentAuthConfig(nodeName + "=node-secret")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Blackbox-Agent-Key", "node-secret")
	req.Header.Set("X-Blackbox-Node-Name", nodeName)

	w := httptest.NewRecorder()
	return req, w, middleware.AgentAuth(config)
}

func TestSetupStatus_NotBootstrapped(t *testing.T) {
	database := newTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	w := httptest.NewRecorder()

	handlers.SetupStatus(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]bool
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp["bootstrapped"])
}

func TestSetupStatus_Bootstrapped(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.User{ID: "01ADMIN", Username: "admin", IsAdmin: true})

	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	w := httptest.NewRecorder()

	handlers.SetupStatus(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]bool
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.True(t, resp["bootstrapped"])
}
