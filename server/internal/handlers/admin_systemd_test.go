package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/stretchr/testify/require"
)

func TestGetSystemdSettings_ReturnsEmptyMapWhenNoneConfigured(t *testing.T) {
	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/settings/systemd", nil)
	w := httptest.NewRecorder()
	handlers.GetSystemdSettings(database)(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string][]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Empty(t, resp)
}

func TestUpdateSystemdSettings_UpsertsUnitList(t *testing.T) {
	database := newTestDB(t)
	body := `{"units":["nginx.service","postgres.service"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/systemd/node-01",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withChiParam(req, "node_name", "node-01")
	w := httptest.NewRecorder()
	handlers.UpdateSystemdSettings(database)(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	var config models.SystemdUnitConfig
	require.NoError(t, database.First(&config, "node_name = ?", "node-01").Error)
	var units []string
	require.NoError(t, json.Unmarshal([]byte(config.Units), &units))
	require.Equal(t, []string{"nginx.service", "postgres.service"}, units)
}

func TestUpdateSystemdSettings_OverwritesExistingList(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.SystemdUnitConfig{
		NodeName: "node-01",
		Units:    `["old.service"]`,
	}).Error)

	body := `{"units":["nginx.service"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/systemd/node-01",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withChiParam(req, "node_name", "node-01")
	w := httptest.NewRecorder()
	handlers.UpdateSystemdSettings(database)(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	var config models.SystemdUnitConfig
	require.NoError(t, database.First(&config, "node_name = ?", "node-01").Error)
	var units []string
	require.NoError(t, json.Unmarshal([]byte(config.Units), &units))
	require.Equal(t, []string{"nginx.service"}, units)
}

func TestGetSystemdSettings_ReturnsAllNodes(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.SystemdUnitConfig{NodeName: "node-01", Units: `["nginx.service"]`}).Error)
	require.NoError(t, database.Create(&models.SystemdUnitConfig{NodeName: "node-02", Units: `["redis.service"]`}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/settings/systemd", nil)
	w := httptest.NewRecorder()
	handlers.GetSystemdSettings(database)(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string][]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, []string{"nginx.service"}, resp["node-01"])
	require.Equal(t, []string{"redis.service"}, resp["node-02"])
}
