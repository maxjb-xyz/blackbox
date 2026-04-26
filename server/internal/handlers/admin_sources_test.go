package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newSourcesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.DataSourceInstance{}, &models.Node{}))
	return db
}

func TestListSources_Empty(t *testing.T) {
	db := newSourcesTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sources", nil)
	w := httptest.NewRecorder()
	handlers.ListSources(db)(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, resp, "server")
	require.Contains(t, resp, "nodes")
}

func TestCreateSource_Valid(t *testing.T) {
	db := newSourcesTestDB(t)
	body, _ := json.Marshal(map[string]any{
		"type": "proxmox", "scope": "agent", "node_id": "homelab-01",
		"name": "pve01", "config": map[string]any{"url": "https://pve01:8006", "api_token": "tok", "insecure_skip_verify": false, "poll_interval_seconds": 10},
		"enabled": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sources", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.CreateSource(db)(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var inst models.DataSourceInstance
	require.NoError(t, db.Where("type = ?", "proxmox").First(&inst).Error)
	require.Equal(t, "pve01", inst.Name)
}

func TestCreateSource_UnknownType(t *testing.T) {
	db := newSourcesTestDB(t)
	body, _ := json.Marshal(map[string]any{"type": "banana", "scope": "agent", "name": "x", "config": map[string]any{}})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sources", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.CreateSource(db)(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateSource(t *testing.T) {
	db := newSourcesTestDB(t)
	nodeName := "homelab-01"
	inst := models.DataSourceInstance{
		ID: "testid", Type: "proxmox", Scope: "agent", NodeID: &nodeName,
		Name: "pve01", Config: `{"url":"https://old:8006"}`, Enabled: true,
	}
	require.NoError(t, db.Create(&inst).Error)

	body, _ := json.Marshal(map[string]any{"name": "pve01-updated", "enabled": false, "config": map[string]any{"url": "https://new:8006"}})
	r := chi.NewRouter()
	r.Put("/api/admin/sources/{id}", handlers.UpdateSource(db))
	req := httptest.NewRequest(http.MethodPut, "/api/admin/sources/testid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated models.DataSourceInstance
	require.NoError(t, db.First(&updated, "id = ?", "testid").Error)
	require.Equal(t, "pve01-updated", updated.Name)
	require.False(t, updated.Enabled)
}

func TestDeleteSource(t *testing.T) {
	db := newSourcesTestDB(t)
	require.NoError(t, db.Create(&models.DataSourceInstance{ID: "del1", Type: "proxmox", Scope: "agent", Name: "x", Config: "{}"}).Error)

	r := chi.NewRouter()
	r.Delete("/api/admin/sources/{id}", handlers.DeleteSource(db))
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sources/del1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	var count int64
	db.Model(&models.DataSourceInstance{}).Where("id = ?", "del1").Count(&count)
	require.Equal(t, int64(0), count)
}

func TestListSourceTypes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sources/types", nil)
	w := httptest.NewRecorder()
	handlers.ListSourceTypes()(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var types []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &types))
	require.NotEmpty(t, types)
	found := map[string]bool{}
	for _, t := range types {
		found[t["type"].(string)] = true
	}
	for _, expected := range []string{"systemd", "filewatcher", "proxmox", "webhook_uptime_kuma", "webhook_watchtower"} {
		require.True(t, found[expected], "missing type: "+expected)
	}
}
