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
