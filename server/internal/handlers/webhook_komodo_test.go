package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/middleware"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createKomodoSource(t *testing.T, db *gorm.DB, allowedTypes []string, nodeMap map[string]string) string {
	t.Helper()
	id := ulid.Make().String()
	// Normalize allowed_types to lowercase to match what validateSourceConfig produces.
	normalized := make([]string, len(allowedTypes))
	for i, v := range allowedTypes {
		normalized[i] = strings.ToLower(strings.TrimSpace(v))
	}
	cfg := map[string]any{
		"secret":        "komodo-secret",
		"allowed_types": normalized,
	}
	// Normalize node_map keys and values to match what validateSourceConfig produces.
	if nodeMap != nil {
		normMap := make(map[string]string, len(nodeMap))
		for k, v := range nodeMap {
			normMap[strings.ToLower(strings.TrimSpace(k))] = strings.ToLower(strings.TrimSpace(v))
		}
		cfg["node_map"] = normMap
	}
	cfgBytes, err := json.Marshal(cfg)
	require.NoError(t, err, "marshal cfg")
	now := time.Now().UTC()
	inst := models.DataSourceInstance{
		ID: id, Type: "webhook_komodo", Scope: "server",
		Name: "Komodo", Config: string(cfgBytes),
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&inst).Error)
	return id
}

func requestWithSourceID(t *testing.T, body string, sourceID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/komodo", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.WebhookSourceIDKey(), sourceID)
	return req.WithContext(ctx)
}

func TestWebhookKomodo_AcceptedType_SavesEntry(t *testing.T) {
	db := newTestDB(t)
	sourceID := createKomodoSource(t, db, []string{"BuildFailed"}, nil)

	body := `{"type":"BuildFailed","data":{"name":"my-service","server_name":"prod-komodo"}}`
	req := requestWithSourceID(t, body, sourceID)
	w := httptest.NewRecorder()

	handlers.WebhookKomodo(db, nil, testIncidentChannel(t), nil)(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var entries []types.Entry
	require.NoError(t, db.Find(&entries).Error)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, "komodo", e.Source)
	assert.Equal(t, "my-service", e.Service)
	assert.Equal(t, "buildfailed", e.Event)
	assert.Equal(t, "prod-komodo", e.NodeName)
	assert.Contains(t, e.Content, "my-service")
	assert.NotEmpty(t, e.ID)
}

func TestWebhookKomodo_IgnoredType_Returns204(t *testing.T) {
	db := newTestDB(t)
	sourceID := createKomodoSource(t, db, []string{"BuildFailed"}, nil)

	body := `{"type":"ContainerStateChange","data":{"name":"nginx"}}`
	req := requestWithSourceID(t, body, sourceID)
	w := httptest.NewRecorder()

	handlers.WebhookKomodo(db, nil, testIncidentChannel(t), nil)(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	var entries []types.Entry
	require.NoError(t, db.Find(&entries).Error)
	assert.Empty(t, entries)
}

func TestWebhookKomodo_NodeMap_MapsNodeName(t *testing.T) {
	db := newTestDB(t)
	nodeMap := map[string]string{"prod-komodo": "prod-1"}
	sourceID := createKomodoSource(t, db, []string{"BuildFailed"}, nodeMap)

	body := `{"type":"BuildFailed","data":{"name":"svc","server_name":"prod-komodo"}}`
	req := requestWithSourceID(t, body, sourceID)
	w := httptest.NewRecorder()

	handlers.WebhookKomodo(db, nil, testIncidentChannel(t), nil)(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var entry types.Entry
	require.NoError(t, db.First(&entry).Error)
	assert.Equal(t, "prod-1", entry.NodeName)
}

func TestWebhookKomodo_NodeMap_UnmappedPassesThrough(t *testing.T) {
	db := newTestDB(t)
	nodeMap := map[string]string{"prod-komodo": "prod-1"}
	sourceID := createKomodoSource(t, db, []string{"BuildFailed"}, nodeMap)

	body := `{"type":"BuildFailed","data":{"name":"svc","server_name":"staging-komodo"}}`
	req := requestWithSourceID(t, body, sourceID)
	w := httptest.NewRecorder()

	handlers.WebhookKomodo(db, nil, testIncidentChannel(t), nil)(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var entry types.Entry
	require.NoError(t, db.First(&entry).Error)
	assert.Equal(t, "staging-komodo", entry.NodeName)
}

func TestWebhookKomodo_MissingName_FallsBackToKomodo(t *testing.T) {
	db := newTestDB(t)
	sourceID := createKomodoSource(t, db, []string{"ServerUnreachable"}, nil)

	body := `{"type":"ServerUnreachable","data":{}}`
	req := requestWithSourceID(t, body, sourceID)
	w := httptest.NewRecorder()

	handlers.WebhookKomodo(db, nil, testIncidentChannel(t), nil)(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var entry types.Entry
	require.NoError(t, db.First(&entry).Error)
	assert.Equal(t, "komodo", entry.Service)
	assert.Equal(t, "komodo", entry.NodeName)
}

func TestWebhookKomodo_MetadataContainsKomodoFields(t *testing.T) {
	db := newTestDB(t)
	sourceID := createKomodoSource(t, db, []string{"BuildFailed"}, nil)

	body := `{"type":"BuildFailed","data":{"name":"my-service","server_name":"prod"}}`
	req := requestWithSourceID(t, body, sourceID)
	w := httptest.NewRecorder()

	handlers.WebhookKomodo(db, nil, testIncidentChannel(t), nil)(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var entry types.Entry
	require.NoError(t, db.First(&entry).Error)

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(entry.Metadata), &meta))
	assert.Equal(t, "BuildFailed", meta["komodo.type"])
	assert.Equal(t, "my-service", meta["komodo.name"])
	assert.Equal(t, "prod", meta["komodo.server_name"])
}

func TestWebhookKomodo_MalformedJSON_Returns400(t *testing.T) {
	db := newTestDB(t)
	sourceID := createKomodoSource(t, db, []string{"BuildFailed"}, nil)

	req := requestWithSourceID(t, "{bad json", sourceID)
	w := httptest.NewRecorder()

	handlers.WebhookKomodo(db, nil, testIncidentChannel(t), nil)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
