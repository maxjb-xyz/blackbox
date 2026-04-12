package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type batchResponse struct {
	Accepted []string `json:"accepted"`
	Failed   []struct {
		ID        string `json:"id"`
		Reason    string `json:"reason"`
		Permanent bool   `json:"permanent"`
	} `json:"failed"`
}

func TestAgentPushBatch_SavesAllEntries(t *testing.T) {
	database := newTestDB(t)

	entries := []types.Entry{
		{
			ID:        ulid.Make().String(),
			Timestamp: time.Now().UTC(),
			NodeName:  "homelab-01",
			Source:    "docker",
			Service:   "nginx",
			Event:     "start",
			Content:   "Container nginx started",
		},
		{
			ID:        ulid.Make().String(),
			Timestamp: time.Now().UTC(),
			NodeName:  "homelab-01",
			Source:    "docker",
			Service:   "redis",
			Event:     "stop",
			Content:   "Container redis stopped",
		},
	}

	req, w, authMiddleware := authenticatedBatchRequest(t, entries, "homelab-01")
	authMiddleware(handlers.AgentPushBatch(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp batchResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp.Accepted, 2)
	assert.Empty(t, resp.Failed)

	var count int64
	require.NoError(t, database.Model(&types.Entry{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestAgentPushBatch_PartialFailure_MissingID(t *testing.T) {
	database := newTestDB(t)

	goodID := ulid.Make().String()
	entries := []types.Entry{
		{
			ID:        goodID,
			Timestamp: time.Now().UTC(),
			NodeName:  "homelab-01",
			Source:    "docker",
			Service:   "nginx",
			Event:     "start",
			Content:   "Container nginx started",
		},
		{
			Timestamp: time.Now().UTC(),
			NodeName:  "homelab-01",
			Source:    "docker",
			Service:   "redis",
			Event:     "stop",
			Content:   "Container redis stopped",
		},
	}

	req, w, authMiddleware := authenticatedBatchRequest(t, entries, "homelab-01")
	authMiddleware(handlers.AgentPushBatch(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp batchResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp.Accepted, 1)
	assert.Equal(t, goodID, resp.Accepted[0])
	require.Len(t, resp.Failed, 1)
	assert.True(t, resp.Failed[0].Permanent, "missing-ID failure should be permanent")

	var count int64
	require.NoError(t, database.Model(&types.Entry{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAgentPushBatch_AcceptsExactlyMaxBatchSize(t *testing.T) {
	database := newTestDB(t)

	entries := make([]types.Entry, 200)
	for i := range entries {
		entries[i] = types.Entry{ID: ulid.Make().String(), Source: "docker", Service: "nginx", Event: "start"}
	}

	req, w, authMiddleware := authenticatedBatchRequest(t, entries, "homelab-01")
	authMiddleware(handlers.AgentPushBatch(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestAgentPushBatch_RejectsTooLarge(t *testing.T) {
	database := newTestDB(t)

	entries := make([]types.Entry, 201)
	for i := range entries {
		entries[i] = types.Entry{ID: ulid.Make().String(), Source: "docker", Service: "nginx", Event: "start"}
	}

	req, w, authMiddleware := authenticatedBatchRequest(t, entries, "homelab-01")
	authMiddleware(handlers.AgentPushBatch(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAgentPushBatch_RejectsUnauthorized(t *testing.T) {
	database := newTestDB(t)
	req, w, _ := authenticatedBatchRequest(t, []types.Entry{}, "homelab-01")
	handlers.AgentPushBatch(database, nil, testIncidentChannel(t), nil).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAgentPushBatch_RestartViaReplaceID(t *testing.T) {
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

	entries := []types.Entry{
		{
			ID:        ulid.Make().String(), // distinct from stop.ID
			ReplaceID: stop.ID,
			Timestamp: time.Now().UTC(),
			NodeName:  "homelab-01",
			Source:    "docker",
			Service:   "nginx",
			Event:     "restart",
			Content:   "Container restarted: nginx",
			Metadata:  `{"raw_events":[]}`,
		},
	}

	req, w, authMiddleware := authenticatedBatchRequest(t, entries, "homelab-01")
	authMiddleware(handlers.AgentPushBatch(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp batchResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp.Accepted, 1)
	assert.Empty(t, resp.Failed)

	var saved types.Entry
	require.NoError(t, database.First(&saved, "id = ?", stop.ID).Error)
	assert.Equal(t, "restart", saved.Event)

	// Restart entry must not have been inserted as a new row.
	var count int64
	require.NoError(t, database.Model(&types.Entry{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAgentPushBatch_RestartUpsertWithinBatch(t *testing.T) {
	database := newTestDB(t)

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

	entries := []types.Entry{
		{
			ID:        existing.ID,
			Timestamp: time.Now().UTC(),
			NodeName:  "homelab-01",
			Source:    "docker",
			Service:   "nginx",
			Event:     "restart",
			Content:   "Container restarted: nginx",
			Metadata:  `{"raw_events":[]}`,
		},
	}

	req, w, authMiddleware := authenticatedBatchRequest(t, entries, "homelab-01")
	authMiddleware(handlers.AgentPushBatch(database, nil, testIncidentChannel(t), nil)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var saved types.Entry
	require.NoError(t, database.First(&saved, "id = ?", existing.ID).Error)
	assert.Equal(t, "restart", saved.Event)
}
