package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentPush_RestartViaReplaceID(t *testing.T) {
	database := newTestDB(t)

	stop := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC().Add(-5 * time.Second),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "stop",
		Content:   "Container stopped: nginx",
		Metadata:  `{"raw_events":[]}`,
	}
	require.NoError(t, database.Create(&stop).Error)

	restart := types.Entry{
		ID:        ulid.Make().String(), // distinct from stop.ID
		ReplaceID: stop.ID,
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "restart",
		Content:   "Container restarted: nginx",
		Metadata:  `{"raw_events":[]}`,
	}

	req, w, authMiddleware := authenticatedAgentRequest(t, restart, "homelab-01")
	authMiddleware(handlers.AgentPush(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var saved types.Entry
	require.NoError(t, database.First(&saved, "id = ?", stop.ID).Error)
	assert.Equal(t, "restart", saved.Event)

	// The restart entry itself should NOT have been inserted as a new row.
	var count int64
	require.NoError(t, database.Model(&types.Entry{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAgentPush_RestartFallsBackToEntryID(t *testing.T) {
	database := newTestDB(t)

	// Legacy path: restart.ID == stop.ID, no ReplaceID.
	existing := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC().Add(-5 * time.Second),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "stop",
		Content:   "Container stopped: nginx",
		Metadata:  `{"raw_events":[]}`,
	}
	require.NoError(t, database.Create(&existing).Error)

	restart := types.Entry{
		ID:        existing.ID, // same ID — legacy path
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "restart",
		Content:   "Container restarted: nginx",
		Metadata:  `{"raw_events":[]}`,
	}

	req, w, authMiddleware := authenticatedAgentRequest(t, restart, "homelab-01")
	authMiddleware(handlers.AgentPush(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var saved types.Entry
	require.NoError(t, database.First(&saved, "id = ?", existing.ID).Error)
	assert.Equal(t, "restart", saved.Event)
}

func TestAgentPush_TreatsExactDuplicateAsSuccess(t *testing.T) {
	database := newTestDB(t)

	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "start",
		Content:   "Container nginx started",
		Metadata:  `{"raw_events":[]}`,
	}
	require.NoError(t, database.Create(&entry).Error)

	req, w, authMiddleware := authenticatedAgentRequest(t, entry, "homelab-01")
	authMiddleware(handlers.AgentPush(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var count int64
	require.NoError(t, database.Model(&types.Entry{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAgentPush_RejectsConflictingDuplicate(t *testing.T) {
	database := newTestDB(t)

	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "start",
		Content:   "Container nginx started",
	}
	require.NoError(t, database.Create(&entry).Error)

	conflict := entry
	conflict.Content = "Container nginx restarted"

	req, w, authMiddleware := authenticatedAgentRequest(t, conflict, "homelab-01")
	authMiddleware(handlers.AgentPush(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	var saved types.Entry
	require.NoError(t, database.First(&saved, "id = ?", entry.ID).Error)
	assert.Equal(t, entry.Content, saved.Content)
}
