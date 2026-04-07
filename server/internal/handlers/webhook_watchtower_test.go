package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookWatchtower_SavesEntry(t *testing.T) {
	database := newTestDB(t)

	body := `{"Title":"Watchtower Updates","Message":"Updated containers: my-app (sha256:abc123)","Level":"info"}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/watchtower", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.WebhookWatchtower(database, nil, testIncidentChannel(t))(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var entries []types.Entry
	require.NoError(t, database.Find(&entries).Error)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, "webhook", e.NodeName)
	assert.Equal(t, "webhook", e.Source)
	assert.Equal(t, "watchtower", e.Service)
	assert.Equal(t, "update", e.Event)
	assert.Equal(t, "Updated containers: my-app (sha256:abc123)", e.Content)
	assert.NotEmpty(t, e.ID)

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(e.Metadata), &meta))
	require.Equal(t, []interface{}{"my-app"}, meta["watchtower.services"])
}

func TestWebhookWatchtower_NormalizesServiceAlias(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.ServiceAlias{
		Canonical: "sonarr",
		Alias:     "my-app",
	}).Error)

	body := `{"Title":"Watchtower Updates","Message":"Updated containers","Level":"info"}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/watchtower", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.WebhookWatchtower(database, nil, testIncidentChannel(t))(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var entry types.Entry
	require.NoError(t, database.First(&entry).Error)
	assert.Equal(t, "watchtower", entry.Service)

	body = `{"Title":"Watchtower Updates","Message":"Updated containers: my-app","Level":"info"}`
	req = httptest.NewRequest(http.MethodPost, "/api/webhooks/watchtower", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	handlers.WebhookWatchtower(database, nil, testIncidentChannel(t))(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var entries []types.Entry
	require.NoError(t, database.Where("source = ? AND event = ?", "webhook", "update").Find(&entries).Error)
	require.Len(t, entries, 2)
	for _, candidate := range entries {
		if candidate.Content == "Updated containers: my-app" {
			entry = candidate
			break
		}
	}
	require.Equal(t, "Updated containers: my-app", entry.Content)
	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(entry.Metadata), &meta))
	require.Equal(t, []interface{}{"sonarr"}, meta["watchtower.services"])
}

func TestWebhookWatchtower_RejectsMalformedJSON(t *testing.T) {
	database := newTestDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/watchtower", bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.WebhookWatchtower(database, nil, testIncidentChannel(t))(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var entries []types.Entry
	require.NoError(t, database.Find(&entries).Error)
	assert.Empty(t, entries)
}
