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

func TestAgentPush_SavesEntry(t *testing.T) {
	database := newTestDB(t)

	entry := types.Entry{
		ID:        "01TESTULIDENTRY1",
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "start",
		Content:   "Container nginx started",
	}
	body, _ := json.Marshal(entry)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.AgentPush(database)(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var saved types.Entry
	require.NoError(t, database.First(&saved, "id = ?", entry.ID).Error)
	assert.Equal(t, "homelab-01", saved.NodeName)
	assert.Equal(t, "docker", saved.Source)
}

func TestAgentPush_RejectsMissingID(t *testing.T) {
	database := newTestDB(t)

	entry := types.Entry{NodeName: "node1", Source: "docker"}
	body, _ := json.Marshal(entry)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.AgentPush(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAgentPush_NormalizesServiceAlias(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.ServiceAlias{
		Canonical: "traefik",
		Alias:     "traefik-proxy",
	}).Error)

	entry := types.Entry{
		ID:        "01TESTULIDENTRY2",
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "traefik-proxy",
		Event:     "start",
		Content:   "Container traefik-proxy started",
	}
	body, _ := json.Marshal(entry)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.AgentPush(database)(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var saved types.Entry
	require.NoError(t, database.First(&saved, "id = ?", entry.ID).Error)
	assert.Equal(t, "traefik", saved.Service)
}

func TestAgentPush_RejectsBlankServiceAfterNormalization(t *testing.T) {
	database := newTestDB(t)

	entry := types.Entry{
		ID:        "01TESTULIDENTRY3",
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "   ",
		Event:     "start",
		Content:   "Container started",
	}
	body, _ := json.Marshal(entry)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.AgentPush(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var count int64
	require.NoError(t, database.Model(&types.Entry{}).Count(&count).Error)
	assert.Zero(t, count)
}
