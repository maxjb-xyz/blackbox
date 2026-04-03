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
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentPush_CreatesNode(t *testing.T) {
	database := newTestDB(t)

	startedAt := time.Now().UTC()
	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: startedAt.Add(-24 * time.Hour),
		NodeName:  "homelab-01",
		Source:    "agent",
		Event:     "start",
		Content:   "container nginx started",
		Metadata:  `{"agent_version":"v0.2.1","ip_address":"10.0.0.5","os_info":"Ubuntu 24.04 LTS"}`,
	}
	body, err := json.Marshal(entry)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handlers.AgentPush(database)(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var node models.Node
	require.NoError(t, database.Where("name = ?", "homelab-01").First(&node).Error)
	assert.Equal(t, "homelab-01", node.Name)
	assert.Equal(t, "v0.2.1", node.AgentVersion)
	assert.Equal(t, "10.0.0.5", node.IPAddress)
	assert.Equal(t, "Ubuntu 24.04 LTS", node.OsInfo)
	assert.WithinDuration(t, startedAt, node.LastSeen, 2*time.Second)
}

func TestAgentPush_HeartbeatUpdatesNodeMetadata(t *testing.T) {
	database := newTestDB(t)

	meta := `{"agent_version":"v0.2.1","ip_address":"10.0.0.5","os_info":"Ubuntu 24.04 LTS"}`
	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "agent",
		Event:     "heartbeat",
		Content:   "Blackbox Agent heartbeat",
		Metadata:  meta,
	}
	body, err := json.Marshal(entry)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handlers.AgentPush(database)(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var node models.Node
	require.NoError(t, database.Where("name = ?", "homelab-01").First(&node).Error)
	assert.Equal(t, "v0.2.1", node.AgentVersion)
	assert.Equal(t, "10.0.0.5", node.IPAddress)
	assert.Equal(t, "Ubuntu 24.04 LTS", node.OsInfo)
}

func TestAgentPush_ThrottlesLastSeenUpdates(t *testing.T) {
	database := newTestDB(t)

	baseTime := time.Now().UTC().Add(-5 * time.Second).Round(0)
	require.NoError(t, database.Create(&models.Node{
		ID:       ulid.Make().String(),
		Name:     "homelab-01",
		LastSeen: baseTime,
	}).Error)

	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Event:     "stop",
		Content:   "container nginx stopped",
	}
	body, err := json.Marshal(entry)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handlers.AgentPush(database)(rr, req)

	var node models.Node
	require.NoError(t, database.Where("name = ?", "homelab-01").First(&node).Error)
	assert.True(t, node.LastSeen.Equal(baseTime), "expected LastSeen to be throttled; baseTime=%v got=%v", baseTime, node.LastSeen)
}

func TestAgentPush_StartUpdatesMetadataForExistingNode(t *testing.T) {
	database := newTestDB(t)

	existingSeen := time.Now().UTC().Add(-5 * time.Second).Round(0)
	require.NoError(t, database.Create(&models.Node{
		ID:       ulid.Make().String(),
		Name:     "homelab-01",
		LastSeen: existingSeen,
	}).Error)

	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC().Add(-24 * time.Hour),
		NodeName:  "homelab-01",
		Source:    "agent",
		Event:     "start",
		Content:   "Blackbox Agent started",
		Metadata:  `{"agent_version":"v0.3.0","ip_address":"10.0.0.8","os_info":"Debian 13"}`,
	}
	body, err := json.Marshal(entry)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handlers.AgentPush(database)(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var node models.Node
	require.NoError(t, database.Where("name = ?", "homelab-01").First(&node).Error)
	assert.Equal(t, "v0.3.0", node.AgentVersion)
	assert.Equal(t, "10.0.0.8", node.IPAddress)
	assert.Equal(t, "Debian 13", node.OsInfo)
	assert.WithinDuration(t, time.Now().UTC(), node.LastSeen, 2*time.Second)
}
