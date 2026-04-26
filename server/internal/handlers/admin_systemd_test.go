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

	var inst models.DataSourceInstance
	require.NoError(t, database.Where("type = ? AND node_id = ?", "systemd", "node-01").First(&inst).Error)
	var cfg struct{ Units []string `json:"units"` }
	require.NoError(t, json.Unmarshal([]byte(inst.Config), &cfg))
	require.Equal(t, []string{"nginx.service", "postgres.service"}, cfg.Units)
}

func TestUpdateSystemdSettings_OverwritesExistingList(t *testing.T) {
	database := newTestDB(t)
	nodeName := "node-01"
	existingCfg, _ := json.Marshal(map[string]any{"units": []string{"old.service"}})
	require.NoError(t, database.Create(&models.DataSourceInstance{
		ID: "sys-test-1", Type: "systemd", Scope: "agent", NodeID: &nodeName,
		Name: "Systemd", Config: string(existingCfg), Enabled: true,
	}).Error)

	body := `{"units":["nginx.service"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/systemd/node-01",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withChiParam(req, "node_name", "node-01")
	w := httptest.NewRecorder()
	handlers.UpdateSystemdSettings(database)(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	var inst models.DataSourceInstance
	require.NoError(t, database.Where("type = ? AND node_id = ?", "systemd", "node-01").First(&inst).Error)
	var cfg struct{ Units []string `json:"units"` }
	require.NoError(t, json.Unmarshal([]byte(inst.Config), &cfg))
	require.Equal(t, []string{"nginx.service"}, cfg.Units)
}

func TestUpdateSystemdSettings_RequiresNodeName(t *testing.T) {
	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/systemd",
		bytes.NewBufferString(`{"units":["nginx.service"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.UpdateSystemdSettings(database)(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateSystemdSettings_RejectsInvalidJSON(t *testing.T) {
	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/systemd/node-01",
		bytes.NewBufferString(`{"units":[}`))
	req.Header.Set("Content-Type", "application/json")
	req = withChiParam(req, "node_name", "node-01")
	w := httptest.NewRecorder()

	handlers.UpdateSystemdSettings(database)(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateSystemdSettings_PersistsEmptyUnitList(t *testing.T) {
	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/systemd/node-01",
		bytes.NewBufferString(`{"units":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req = withChiParam(req, "node_name", "node-01")
	w := httptest.NewRecorder()

	handlers.UpdateSystemdSettings(database)(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)

	var inst models.DataSourceInstance
	require.NoError(t, database.Where("type = ? AND node_id = ?", "systemd", "node-01").First(&inst).Error)
	var cfg struct{ Units []string `json:"units"` }
	require.NoError(t, json.Unmarshal([]byte(inst.Config), &cfg))
	require.Empty(t, cfg.Units)
}

func TestUpdateSystemdSettings_DeduplicatesMixedForms(t *testing.T) {
	database := newTestDB(t)
	body := `{"units":["nginx","nginx.service","redis.service","redis"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/systemd/node-01",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withChiParam(req, "node_name", "node-01")
	w := httptest.NewRecorder()
	handlers.UpdateSystemdSettings(database)(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	var inst models.DataSourceInstance
	require.NoError(t, database.Where("type = ? AND node_id = ?", "systemd", "node-01").First(&inst).Error)
	var cfg struct{ Units []string `json:"units"` }
	require.NoError(t, json.Unmarshal([]byte(inst.Config), &cfg))
	require.Equal(t, []string{"nginx.service", "redis.service"}, cfg.Units)
}

func TestUpdateSystemdSettings_NormalizesBareName(t *testing.T) {
	database := newTestDB(t)
	body := `{"units":["nginx","redis.service"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/systemd/node-01",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withChiParam(req, "node_name", "node-01")
	w := httptest.NewRecorder()
	handlers.UpdateSystemdSettings(database)(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	var inst models.DataSourceInstance
	require.NoError(t, database.Where("type = ? AND node_id = ?", "systemd", "node-01").First(&inst).Error)
	var cfg struct{ Units []string `json:"units"` }
	require.NoError(t, json.Unmarshal([]byte(inst.Config), &cfg))
	require.Equal(t, []string{"nginx.service", "redis.service"}, cfg.Units)
}

func TestUpdateSystemdSettings_DottedBareNameGetsSuffix(t *testing.T) {
	database := newTestDB(t)
	body := `{"units":["dbus-org.freedesktop.resolve1"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/systemd/node-01",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = withChiParam(req, "node_name", "node-01")
	w := httptest.NewRecorder()
	handlers.UpdateSystemdSettings(database)(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	var inst models.DataSourceInstance
	require.NoError(t, database.Where("type = ? AND node_id = ?", "systemd", "node-01").First(&inst).Error)
	var cfg struct{ Units []string `json:"units"` }
	require.NoError(t, json.Unmarshal([]byte(inst.Config), &cfg))
	require.Equal(t, []string{"dbus-org.freedesktop.resolve1.service"}, cfg.Units)
}

func TestGetSystemdSettings_ReturnsAllNodes(t *testing.T) {
	database := newTestDB(t)
	node01 := "node-01"
	node02 := "node-02"
	cfg1, _ := json.Marshal(map[string]any{"units": []string{"nginx.service"}})
	cfg2, _ := json.Marshal(map[string]any{"units": []string{"redis.service"}})
	require.NoError(t, database.Create(&models.DataSourceInstance{
		ID: "sys-n1", Type: "systemd", Scope: "agent", NodeID: &node01,
		Name: "Systemd", Config: string(cfg1), Enabled: true,
	}).Error)
	require.NoError(t, database.Create(&models.DataSourceInstance{
		ID: "sys-n2", Type: "systemd", Scope: "agent", NodeID: &node02,
		Name: "Systemd", Config: string(cfg2), Enabled: true,
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/settings/systemd", nil)
	w := httptest.NewRecorder()
	handlers.GetSystemdSettings(database)(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string][]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, []string{"nginx.service"}, resp["node-01"])
	require.Equal(t, []string{"redis.service"}, resp["node-02"])
}
