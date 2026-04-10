package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/hub"
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
	authMiddleware(handlers.AgentPush(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

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
	authMiddleware(handlers.AgentPush(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
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
	authMiddleware(handlers.AgentPush(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

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
	authMiddleware(handlers.AgentPush(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var count int64
	require.NoError(t, database.Model(&types.Entry{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAgentPush_RestartWithNoExistingEntryCreatesNormally(t *testing.T) {
	database := newTestDB(t)

	entry := types.Entry{
		ID:             "01TESTRestart0001",
		Timestamp:      time.Now().UTC(),
		NodeName:       "homelab-01",
		Source:         "docker",
		Service:        "nginx",
		ComposeService: "web",
		Event:          "restart",
		Content:        "Container restarted: nginx",
	}
	req, w, authMiddleware := authenticatedAgentRequest(t, entry, "homelab-01")
	authMiddleware(handlers.AgentPush(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var saved types.Entry
	require.NoError(t, database.First(&saved, "id = ?", entry.ID).Error)
	assert.Equal(t, "restart", saved.Event)
	assert.Equal(t, "web", saved.ComposeService)
}

func TestAgentPush_RestartReplacesExistingStopEntry(t *testing.T) {
	database := newTestDB(t)

	existing := types.Entry{
		ID:             "01STOPENTRY00001",
		Timestamp:      time.Now().UTC().Add(-5 * time.Second),
		NodeName:       "homelab-01",
		Source:         "docker",
		Service:        "nginx",
		ComposeService: "web",
		Event:          "stop",
		Content:        "Container stopped: nginx",
		Metadata:       `{"raw_events":[]}`,
	}
	require.NoError(t, database.Create(&existing).Error)

	entry := types.Entry{
		ID:             existing.ID,
		Timestamp:      time.Now().UTC(),
		NodeName:       "homelab-01",
		Source:         "docker",
		Service:        "nginx",
		ComposeService: "web",
		Event:          "restart",
		Content:        "Container restarted: nginx",
		Metadata:       `{"raw_events":[]}`,
	}
	req, w, authMiddleware := authenticatedAgentRequest(t, entry, "homelab-01")
	authMiddleware(handlers.AgentPush(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var saved types.Entry
	require.NoError(t, database.First(&saved, "id = ?", entry.ID).Error)
	assert.Equal(t, "restart", saved.Event)
	assert.Equal(t, "Container restarted: nginx", saved.Content)
	assert.Equal(t, "web", saved.ComposeService)
}

func TestAgentPush_RestartBroadcastsEntryReplaced(t *testing.T) {
	database := newTestDB(t)
	eventHub := hub.New()

	require.NoError(t, database.Create(&models.Node{
		ID:       "01NODE000000000001",
		Name:     "homelab-01",
		LastSeen: time.Now().UTC(),
		Status:   "online",
	}).Error)

	existing := types.Entry{
		ID:             "01STOPENTRY00002",
		Timestamp:      time.Now().UTC().Add(-5 * time.Second),
		NodeName:       "homelab-01",
		Source:         "docker",
		Service:        "nginx",
		ComposeService: "web",
		Event:          "stop",
		Content:        "Container stopped: nginx",
		Metadata:       `{"raw_events":[]}`,
	}
	require.NoError(t, database.Create(&existing).Error)

	_, ch, _, unsub, err := eventHub.Subscribe("u1", "10.0.0.1")
	require.NoError(t, err)
	defer unsub()

	entry := types.Entry{
		ID:             existing.ID,
		Timestamp:      time.Now().UTC(),
		NodeName:       "homelab-01",
		Source:         "docker",
		Service:        "nginx",
		ComposeService: "web",
		Event:          "restart",
		Content:        "Container restarted: nginx",
		Metadata:       `{"raw_events":[]}`,
	}
	req, w, authMiddleware := authenticatedAgentRequest(t, entry, "homelab-01")
	authMiddleware(handlers.AgentPush(database, eventHub, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	select {
	case msg := <-ch:
		var payload struct {
			Type string `json:"type"`
			Data struct {
				OldID string      `json:"old_id"`
				Entry types.Entry `json:"entry"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(msg, &payload))
		assert.Equal(t, "entry_replaced", payload.Type)
		assert.Equal(t, existing.ID, payload.Data.OldID)
		assert.Equal(t, existing.ID, payload.Data.Entry.ID)
		assert.Equal(t, "restart", payload.Data.Entry.Event)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected websocket broadcast")
	}
}
