package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListNodes_Empty(t *testing.T) {
	database := newTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	rr := httptest.NewRecorder()

	handlers.ListNodes(database)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var result []map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &result))
	assert.Len(t, result, 0)
}

func TestListNodes_StatusOnlineOffline(t *testing.T) {
	database := newTestDB(t)

	require.NoError(t, database.Create(&models.Node{
		ID:       ulid.Make().String(),
		Name:     "online-node",
		LastSeen: time.Now().UTC().Add(-2 * time.Minute),
	}).Error)
	require.NoError(t, database.Create(&models.Node{
		ID:       ulid.Make().String(),
		Name:     "offline-node",
		LastSeen: time.Now().UTC().Add(-10 * time.Minute),
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	rr := httptest.NewRecorder()

	handlers.ListNodes(database)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var nodes []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &nodes))
	require.Len(t, nodes, 2)

	statusByName := map[string]string{}
	for _, node := range nodes {
		statusByName[node.Name] = node.Status
	}
	assert.Equal(t, "online", statusByName["online-node"])
	assert.Equal(t, "offline", statusByName["offline-node"])
}

func TestListNodes_ExplicitOfflineOverridesFreshLastSeen(t *testing.T) {
	database := newTestDB(t)

	require.NoError(t, database.Create(&models.Node{
		ID:       ulid.Make().String(),
		Name:     "graceful-stop-node",
		LastSeen: time.Now().UTC().Add(-30 * time.Second),
		Status:   "offline",
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	rr := httptest.NewRecorder()

	handlers.ListNodes(database)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var nodes []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &nodes))
	require.Len(t, nodes, 1)
	assert.Equal(t, "offline", nodes[0].Status)
}

func TestListNodes_IncludesQueueVisibility(t *testing.T) {
	database := newTestDB(t)
	oldest := time.Now().UTC().Add(-4 * time.Minute).Truncate(time.Millisecond)
	require.NoError(t, database.Create(&models.Node{
		ID:            ulid.Make().String(),
		Name:          "queued-node",
		LastSeen:      time.Now().UTC(),
		QueueReported: true,
		QueueDepth:    3,
		QueueOldestAt: &oldest,
		QueueRetries:  5,
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	rr := httptest.NewRecorder()
	handlers.ListNodes(database)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var nodes []struct {
		QueueReported bool       `json:"queue_reported"`
		QueueDepth    int        `json:"queue_depth"`
		QueueOldestAt *time.Time `json:"queue_oldest_at"`
		QueueRetries  int        `json:"queue_retry_count"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &nodes))
	require.Len(t, nodes, 1)
	assert.True(t, nodes[0].QueueReported)
	assert.Equal(t, 3, nodes[0].QueueDepth)
	assert.Equal(t, 5, nodes[0].QueueRetries)
	require.NotNil(t, nodes[0].QueueOldestAt)
	assert.WithinDuration(t, oldest, *nodes[0].QueueOldestAt, time.Millisecond)
}
