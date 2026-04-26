package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbpkg "blackbox/server/internal/db"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newSourcesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := dbpkg.Init(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestListSources_Empty(t *testing.T) {
	db := newSourcesTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sources", nil)
	w := httptest.NewRecorder()
	// This relies on the handler injecting legacy default capabilities for nodes with empty stored capabilities.
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
		"type": "filewatcher", "scope": "agent", "node_id": "homelab-01",
		"name": "watcher", "config": map[string]any{"redact_secrets": false},
		"enabled": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sources", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.CreateSource(db)(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var inst models.DataSourceInstance
	require.NoError(t, db.Where("type = ?", "filewatcher").First(&inst).Error)
	require.Equal(t, "watcher", inst.Name)
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
		ID: "testid", Type: "filewatcher", Scope: "agent", NodeID: &nodeName,
		Name: "watcher", Config: `{"redact_secrets":true}`, Enabled: true,
	}
	require.NoError(t, db.Create(&inst).Error)

	body, _ := json.Marshal(map[string]any{"name": "watcher-updated", "enabled": false, "config": map[string]any{"redact_secrets": false}})
	r := chi.NewRouter()
	r.Put("/api/admin/sources/{id}", handlers.UpdateSource(db))
	req := httptest.NewRequest(http.MethodPut, "/api/admin/sources/testid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated models.DataSourceInstance
	require.NoError(t, db.First(&updated, "id = ?", "testid").Error)
	require.Equal(t, "watcher-updated", updated.Name)
	require.False(t, updated.Enabled)
}

func TestUpdateSource_PreservesSensitiveConfigWhenOmitted(t *testing.T) {
	db := newSourcesTestDB(t)
	inst := models.DataSourceInstance{
		ID: "testid", Type: "webhook_uptime_kuma", Scope: "server",
		Name: "UK", Config: `{"secret":"keep-me","note":"existing"}`,
		Enabled: true,
	}
	require.NoError(t, db.Create(&inst).Error)

	body, err := json.Marshal(map[string]any{
		"config": map[string]any{"note": "updated"},
	})
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Put("/api/admin/sources/{id}", handlers.UpdateSource(db))
	req := httptest.NewRequest(http.MethodPut, "/api/admin/sources/testid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated models.DataSourceInstance
	require.NoError(t, db.First(&updated, "id = ?", "testid").Error)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(updated.Config), &cfg))
	require.Equal(t, "updated", cfg["note"])
	require.Equal(t, "keep-me", cfg["secret"])
}

func TestUpdateSource_PreservesSensitiveConfigWhenNullOrNonString(t *testing.T) {
	db := newSourcesTestDB(t)
	inst := models.DataSourceInstance{
		ID: "testid", Type: "webhook_uptime_kuma", Scope: "server",
		Name: "UK", Config: `{"secret":"keep-me","note":"existing"}`,
		Enabled: true,
	}
	require.NoError(t, db.Create(&inst).Error)

	for name, body := range map[string]string{
		"null":       `{"config":{"secret":null,"note":"updated"}}`,
		"non-string": `{"config":{"secret":123,"note":"updated"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, db.Model(&models.DataSourceInstance{}).Where("id = ?", "testid").Update("config", `{"secret":"keep-me","note":"existing"}`).Error)

			r := chi.NewRouter()
			r.Put("/api/admin/sources/{id}", handlers.UpdateSource(db))
			req := httptest.NewRequest(http.MethodPut, "/api/admin/sources/testid", bytes.NewReader([]byte(body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var updated models.DataSourceInstance
			require.NoError(t, db.First(&updated, "id = ?", "testid").Error)
			var cfg map[string]any
			require.NoError(t, json.Unmarshal([]byte(updated.Config), &cfg))
			require.Equal(t, "updated", cfg["note"])
			require.Equal(t, "keep-me", cfg["secret"])
		})
	}
}

func TestUpdateSource_DisablingWebhookClearsRuntimeSecretCache(t *testing.T) {
	db := newSourcesTestDB(t)
	inst := models.DataSourceInstance{
		ID: "testid", Type: "webhook_uptime_kuma", Scope: "server",
		Name: "UK", Config: `{"secret":"keep-me"}`, Enabled: true,
	}
	require.NoError(t, db.Create(&inst).Error)
	require.Equal(t, "keep-me", handlers.PrimeWebhookSecretCache(db, "webhook_uptime_kuma", ""))

	r := chi.NewRouter()
	r.Put("/api/admin/sources/{id}", handlers.UpdateSource(db))
	req := httptest.NewRequest(http.MethodPut, "/api/admin/sources/testid", bytes.NewReader([]byte(`{"enabled":false}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.Equal(t, "", handlers.GetCachedWebhookSecret(db, "webhook_uptime_kuma", ""))
}

func TestDeleteSource(t *testing.T) {
	db := newSourcesTestDB(t)
	require.NoError(t, db.Create(&models.DataSourceInstance{ID: "del1", Type: "filewatcher", Scope: "agent", Name: "x", Config: "{}"}).Error)

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

func TestDeleteSource_ClearsRuntimeWebhookSecretCache(t *testing.T) {
	db := newSourcesTestDB(t)
	require.NoError(t, db.Create(&models.DataSourceInstance{
		ID: "wh1", Type: "webhook_uptime_kuma", Scope: "server",
		Name: "UK", Config: `{"secret":"db-secret"}`, Enabled: true,
	}).Error)
	require.Equal(t, "db-secret", handlers.PrimeWebhookSecretCache(db, "webhook_uptime_kuma", ""))

	r := chi.NewRouter()
	r.Delete("/api/admin/sources/{id}", handlers.DeleteSource(db))
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sources/wh1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	require.Equal(t, "", handlers.GetCachedWebhookSecret(db, "webhook_uptime_kuma", ""))
}

func TestCreateSource_SingletonEnforced(t *testing.T) {
	db := newSourcesTestDB(t)
	// Create first systemd instance
	nodeName := "homelab-01"
	require.NoError(t, db.Create(&models.DataSourceInstance{
		ID: "sys1", Type: "systemd", Scope: "agent", NodeID: &nodeName, Name: "Systemd", Config: "{}",
	}).Error)

	// Attempt to create second systemd instance for same node — must fail 409
	body, _ := json.Marshal(map[string]any{
		"type": "systemd", "scope": "agent", "node_id": "homelab-01", "name": "Systemd2", "config": map[string]any{"units": []string{}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sources", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.CreateSource(db)(w, req)
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestCreateSource_ServerScopedRejectsNodeID(t *testing.T) {
	db := newSourcesTestDB(t)
	body, _ := json.Marshal(map[string]any{
		"type": "webhook_uptime_kuma", "scope": "server", "node_id": "homelab-01",
		"name": "UK", "config": map[string]any{"secret": "x"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sources", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.CreateSource(db)(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateSource_AgentScopedRequiresNodeID(t *testing.T) {
	db := newSourcesTestDB(t)
	body, _ := json.Marshal(map[string]any{
		"type": "systemd", "scope": "agent", "name": "Sys", "config": map[string]any{"units": []string{}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sources", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.CreateSource(db)(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetWebhookSecret_DBHit(t *testing.T) {
	db := newSourcesTestDB(t)
	require.NoError(t, db.Create(&models.DataSourceInstance{
		ID: "wh1", Type: "webhook_uptime_kuma", Scope: "server",
		Name: "UK", Config: `{"secret":"db-secret"}`, Enabled: true,
	}).Error)
	result := handlers.GetWebhookSecret(db, "webhook_uptime_kuma", "env-secret")
	require.Equal(t, "db-secret", result)
}

func TestGetWebhookSecret_MissingSourceDisablesWebhook(t *testing.T) {
	db := newSourcesTestDB(t)
	result := handlers.GetWebhookSecret(db, "webhook_uptime_kuma", "env-secret")
	require.Equal(t, "", result)
}

func TestGetWebhookSecret_EmptyDBSecretDisablesWebhook(t *testing.T) {
	db := newSourcesTestDB(t)
	require.NoError(t, db.Create(&models.DataSourceInstance{
		ID: "wh2", Type: "webhook_uptime_kuma", Scope: "server",
		Name: "UK", Config: `{"secret":""}`, Enabled: true,
	}).Error)
	result := handlers.GetWebhookSecret(db, "webhook_uptime_kuma", "env-secret")
	require.Equal(t, "", result)
}

func TestGetWebhookSecret_DisabledOrMissingSourceDisablesWebhook(t *testing.T) {
	db := newSourcesTestDB(t)
	require.NoError(t, db.Create(&models.DataSourceInstance{
		ID: "wh1", Type: "webhook_uptime_kuma", Scope: "server",
		Name: "UK", Config: `{"secret":"db-secret"}`, Enabled: false,
	}).Error)
	require.NoError(t, db.Model(&models.DataSourceInstance{}).Where("id = ?", "wh1").Update("enabled", false).Error)
	require.Equal(t, "", handlers.GetWebhookSecret(db, "webhook_uptime_kuma", "env-secret"))

	require.NoError(t, db.Delete(&models.DataSourceInstance{}, "id = ?", "wh1").Error)
	require.Equal(t, "", handlers.GetWebhookSecret(db, "webhook_uptime_kuma", "env-secret"))
}

// This intentionally bypasses dbpkg.Init and newSourcesTestDB because the production
// uniqueness constraint would prevent creating two server-scoped webhook rows, which
// would make handlers.GetWebhookSecret impossible to verify for oldest-row preference.
func TestGetWebhookSecret_PrefersOldestEnabledInstance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.DataSourceInstance{}))
	now := time.Now().UTC()
	require.NoError(t, db.Create(&models.DataSourceInstance{
		ID: "wh-new", Type: "webhook_uptime_kuma", Scope: "server",
		Name: "UK new", Config: `{"secret":"newer-secret"}`, Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&models.DataSourceInstance{
		ID: "wh-old", Type: "webhook_uptime_kuma", Scope: "server",
		Name: "UK old", Config: `{"secret":"older-secret"}`, Enabled: true,
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}).Error)

	result := handlers.GetWebhookSecret(db, "webhook_uptime_kuma", "env-secret")
	require.Equal(t, "older-secret", result)
}

func TestListSources_WebhookSecretRedacted(t *testing.T) {
	db := newSourcesTestDB(t)
	require.NoError(t, db.Create(&models.DataSourceInstance{
		ID: "wh1", Type: "webhook_uptime_kuma", Scope: "server",
		Name: "UK", Config: `{"secret":"supersecret"}`, Enabled: true,
	}).Error)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sources", nil)
	w := httptest.NewRecorder()
	handlers.ListSources(db)(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	require.NotContains(t, body, "supersecret")
}

func TestListSources_OrphansIncluded(t *testing.T) {
	db := newSourcesTestDB(t)
	nodeName := "homelab-01"
	require.NoError(t, db.Create(&models.Node{
		ID: "n1", Name: nodeName, Capabilities: "[]",
	}).Error)
	require.NoError(t, db.Create(&models.DataSourceInstance{
		ID: "fw1", Type: "filewatcher", Scope: "agent", NodeID: &nodeName,
		Name: "Watcher", Config: `{"redact_secrets":true}`, Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&models.DataSourceInstance{
		ID: "orphan1", Type: "filewatcher", Scope: "agent",
		Name: "Orphan", Config: `{"redact_secrets":true}`, Enabled: true,
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/sources", nil)
	w := httptest.NewRecorder()
	handlers.ListSources(db)(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Nodes map[string]struct {
			Capabilities []string                    `json:"capabilities"`
			Sources      []models.DataSourceInstance `json:"sources"`
		} `json:"nodes"`
		Orphans []models.DataSourceInstance `json:"orphans"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Nodes[nodeName].Capabilities)
	require.Contains(t, resp.Nodes[nodeName].Capabilities, "filewatcher")
	require.Len(t, resp.Nodes[nodeName].Sources, 1)
	require.Len(t, resp.Orphans, 1)
	require.Equal(t, "orphan1", resp.Orphans[0].ID)
}

func TestCreateSource_InvalidConfigRejected(t *testing.T) {
	db := newSourcesTestDB(t)
	// Send a non-object JSON value as config
	raw := []byte(`{"type":"filewatcher","scope":"agent","node_id":"homelab-01","name":"watcher","config":42,"enabled":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sources", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.CreateSource(db)(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateSource_RejectsVirtualDockerSource(t *testing.T) {
	db := newSourcesTestDB(t)
	raw := []byte(`{"type":"docker","scope":"agent","node_id":"homelab-01","name":"docker","config":{},"enabled":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sources", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.CreateSource(db)(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "virtual")
}

func TestCreateSource_FileWatcherRequiresRedactSecrets(t *testing.T) {
	db := newSourcesTestDB(t)
	raw := []byte(`{"type":"filewatcher","scope":"agent","node_id":"homelab-01","name":"watcher","config":{},"enabled":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sources", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.CreateSource(db)(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "redact_secrets is required")
}

func TestCreateSource_NullConfigRejected(t *testing.T) {
	db := newSourcesTestDB(t)
	raw := []byte(`{"type":"filewatcher","scope":"agent","node_id":"homelab-01","name":"watcher","config":null,"enabled":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sources", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.CreateSource(db)(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateSource_NullConfigRejected(t *testing.T) {
	db := newSourcesTestDB(t)
	nodeName := "homelab-01"
	inst := models.DataSourceInstance{
		ID: "testid", Type: "filewatcher", Scope: "agent", NodeID: &nodeName,
		Name: "watcher", Config: `{"redact_secrets":true}`, Enabled: true,
	}
	require.NoError(t, db.Create(&inst).Error)

	r := chi.NewRouter()
	r.Put("/api/admin/sources/{id}", handlers.UpdateSource(db))
	req := httptest.NewRequest(http.MethodPut, "/api/admin/sources/testid", bytes.NewReader([]byte(`{"config":null}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateSource_EmptyNameRejected(t *testing.T) {
	db := newSourcesTestDB(t)
	nodeName := "homelab-01"
	inst := models.DataSourceInstance{
		ID: "testid", Type: "filewatcher", Scope: "agent", NodeID: &nodeName,
		Name: "watcher", Config: `{"redact_secrets":true}`, Enabled: true,
	}
	require.NoError(t, db.Create(&inst).Error)

	r := chi.NewRouter()
	r.Put("/api/admin/sources/{id}", handlers.UpdateSource(db))
	req := httptest.NewRequest(http.MethodPut, "/api/admin/sources/testid", bytes.NewReader([]byte(`{"name":""}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateSource_FileWatcherRequiresRedactSecrets(t *testing.T) {
	db := newSourcesTestDB(t)
	nodeName := "homelab-01"
	inst := models.DataSourceInstance{
		ID: "testid", Type: "filewatcher", Scope: "agent", NodeID: &nodeName,
		Name: "watcher", Config: `{"redact_secrets":true}`, Enabled: true,
	}
	require.NoError(t, db.Create(&inst).Error)

	r := chi.NewRouter()
	r.Put("/api/admin/sources/{id}", handlers.UpdateSource(db))
	req := httptest.NewRequest(http.MethodPut, "/api/admin/sources/testid", bytes.NewReader([]byte(`{"config":{}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "redact_secrets is required")
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
	for _, typ := range types {
		rawType, ok := typ["type"]
		require.True(t, ok, "missing type field")
		sourceType, ok := rawType.(string)
		require.True(t, ok, "type field must be a string")
		found[sourceType] = true
	}
	for _, expected := range []string{"systemd", "filewatcher", "webhook_uptime_kuma", "webhook_watchtower"} {
		require.True(t, found[expected], "missing type: "+expected)
	}
	require.True(t, found["docker"], "missing type: docker")
	require.GreaterOrEqual(t, len(found), 5)
}
