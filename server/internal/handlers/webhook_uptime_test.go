package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookUptime_DownEvent_NoCorrelation(t *testing.T) {
	database := newTestDB(t)

	body := `{
		"heartbeat": {"status": 0, "time": "2026-04-02T02:00:00Z", "msg": "Connection refused"},
		"monitor":   {"name": "my-app"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/uptime", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.WebhookUptime(database)(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var entries []types.Entry
	require.NoError(t, database.Find(&entries).Error)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, "webhook", e.NodeName)
	assert.Equal(t, "webhook", e.Source)
	assert.Equal(t, "my-app", e.Service)
	assert.Equal(t, "down", e.Event)
	assert.Contains(t, e.Content, "my-app")
	assert.Contains(t, e.Content, "Connection refused")
	assert.Empty(t, e.CorrelatedID)

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(e.Metadata), &meta))
	assert.Equal(t, "my-app", meta["monitor"])
	assert.Equal(t, "down", meta["status"])
	assert.Nil(t, meta["possible_cause"])
}

func TestWebhookUptime_DownEvent_WithCorrelation(t *testing.T) {
	database := newTestDB(t)

	webhookTime := time.Date(2026, 4, 2, 2, 0, 0, 0, time.UTC)
	agentEntry := types.Entry{
		ID:        "01AGENTENTRY000001",
		Timestamp: webhookTime.Add(-60 * time.Second),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "my-app",
		Event:     "die",
		Content:   "container 'my-app' died (exit code: 137)",
	}
	require.NoError(t, database.Create(&agentEntry).Error)

	body := `{
		"heartbeat": {"status": 0, "time": "2026-04-02T02:00:00Z", "msg": "Connection refused"},
		"monitor":   {"name": "my-app"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/uptime", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.WebhookUptime(database)(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var entries []types.Entry
	require.NoError(t, database.Where("source = ?", "webhook").Find(&entries).Error)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, "01AGENTENTRY000001", e.CorrelatedID)

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(e.Metadata), &meta))
	assert.Equal(t, "my-app", meta["monitor"])
	assert.Equal(t, "down", meta["status"])
	assert.Equal(t, "container 'my-app' died (exit code: 137)", meta["possible_cause"])
	assert.Equal(t, "homelab-01", meta["cause_node"])
	assert.Equal(t, "die", meta["cause_event"])
	assert.Equal(t, "01AGENTENTRY000001", meta["cause_entry_id"])
}

func TestWebhookUptime_UpEvent(t *testing.T) {
	database := newTestDB(t)

	body := `{
		"heartbeat": {"status": 1, "time": "2026-04-02T02:05:00Z", "msg": "OK: 200 OK - 45ms"},
		"monitor":   {"name": "my-app"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/uptime", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.WebhookUptime(database)(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var entries []types.Entry
	require.NoError(t, database.Find(&entries).Error)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, "up", e.Event)
	assert.Contains(t, e.Content, "recovered")
	assert.Empty(t, e.CorrelatedID)

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(e.Metadata), &meta))
	assert.Equal(t, "my-app", meta["monitor"])
	assert.Equal(t, "up", meta["status"])
	assert.Equal(t, "OK: 200 OK - 45ms", meta["recovery_msg"])
}

func TestWebhookUptime_UpEvent_AddsOutageDurationAndNormalizesService(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.ServiceAlias{
		Canonical: "traefik",
		Alias:     "traefik-proxy",
	}).Error)

	downAt := time.Date(2026, 4, 2, 2, 0, 0, 0, time.UTC)
	require.NoError(t, database.Create(&types.Entry{
		ID:        "01DOWNENTRY0000001",
		Timestamp: downAt,
		NodeName:  "webhook",
		Source:    "webhook",
		Service:   "traefik",
		Event:     "down",
		Content:   "Monitor 'traefik-proxy' is down: timeout",
		Metadata:  `{"monitor":"traefik-proxy","status":"down"}`,
	}).Error)

	body := `{
		"heartbeat": {"status": 1, "time": "2026-04-02T02:05:30Z", "msg": "OK"},
		"monitor":   {"name": "traefik-proxy"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/uptime", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.WebhookUptime(database)(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var entry types.Entry
	require.NoError(t, database.Where("event = ?", "up").First(&entry).Error)
	assert.Equal(t, "traefik", entry.Service)

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(entry.Metadata), &meta))
	assert.Equal(t, float64(330), meta["duration_seconds"])
	assert.Equal(t, "2026-04-02T02:00:00Z", meta["down_since"])
}

func TestWebhookUptime_MissingMonitorName(t *testing.T) {
	database := newTestDB(t)

	body := `{"heartbeat": {"status": 0, "msg": "down"}, "monitor": {}}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/uptime", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.WebhookUptime(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var entries []types.Entry
	require.NoError(t, database.Find(&entries).Error)
	assert.Empty(t, entries)
}

func TestWebhookUptime_MalformedJSON(t *testing.T) {
	database := newTestDB(t)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/uptime", bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.WebhookUptime(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWebhookUptime_BadTimestampFallsBackAndSetsFlag(t *testing.T) {
	database := newTestDB(t)

	body := `{
		"heartbeat": {"status": 0, "time": "not-a-timestamp", "msg": "down"},
		"monitor":   {"name": "my-app"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/uptime", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	before := time.Now().UTC()
	handlers.WebhookUptime(database)(w, req)
	after := time.Now().UTC()

	assert.Equal(t, http.StatusCreated, w.Code)

	var entries []types.Entry
	require.NoError(t, database.Find(&entries).Error)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.True(t, e.Timestamp.After(before) || e.Timestamp.Equal(before))
	assert.True(t, e.Timestamp.Before(after) || e.Timestamp.Equal(after))

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(e.Metadata), &meta))
	assert.Equal(t, true, meta["time_fallback"])
}
