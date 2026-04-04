package handlers_test

import (
	"bytes"
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

	handlers.WebhookWatchtower(database, nil)(w, req)

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
}

func TestWebhookWatchtower_NormalizesServiceAlias(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.ServiceAlias{
		Canonical: "ops-watchtower",
		Alias:     "watchtower",
	}).Error)

	body := `{"Title":"Watchtower Updates","Message":"Updated containers","Level":"info"}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/watchtower", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.WebhookWatchtower(database, nil)(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var entry types.Entry
	require.NoError(t, database.First(&entry).Error)
	assert.Equal(t, "ops-watchtower", entry.Service)
}

func TestWebhookWatchtower_RejectsMalformedJSON(t *testing.T) {
	database := newTestDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/watchtower", bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.WebhookWatchtower(database, nil)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var entries []types.Entry
	require.NoError(t, database.Find(&entries).Error)
	assert.Empty(t, entries)
}
