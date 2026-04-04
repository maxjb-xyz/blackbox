package handlers_test

import (
	"net/http"
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
	req, w, authMiddleware := authenticatedAgentRequest(t, entry, "homelab-01")
	authMiddleware(handlers.AgentPush(database)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var saved types.Entry
	require.NoError(t, database.First(&saved, "id = ?", entry.ID).Error)
	assert.Equal(t, "homelab-01", saved.NodeName)
	assert.Equal(t, "docker", saved.Source)
}

func TestAgentPush_RejectsMissingID(t *testing.T) {
	database := newTestDB(t)

	entry := types.Entry{NodeName: "node1", Source: "docker"}
	req, w, authMiddleware := authenticatedAgentRequest(t, entry, "node1")
	authMiddleware(handlers.AgentPush(database)).ServeHTTP(w, req)

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
	req, w, authMiddleware := authenticatedAgentRequest(t, entry, "homelab-01")
	authMiddleware(handlers.AgentPush(database)).ServeHTTP(w, req)

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
	req, w, authMiddleware := authenticatedAgentRequest(t, entry, "homelab-01")
	authMiddleware(handlers.AgentPush(database)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var count int64
	require.NoError(t, database.Model(&types.Entry{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAgentPush_AllowsBlankServiceForAgentMetaEvents(t *testing.T) {
	database := newTestDB(t)

	entry := types.Entry{
		ID:        "01TESTULIDENTRY4",
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "agent",
		Service:   "   ",
		Event:     "heartbeat",
		Content:   "Blackbox Agent heartbeat",
	}
	req, w, authMiddleware := authenticatedAgentRequest(t, entry, "homelab-01")
	authMiddleware(handlers.AgentPush(database)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var count int64
	require.NoError(t, database.Model(&types.Entry{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
