package handlers_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/hub"
	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartNodeStatusMonitor_MarksStaleNodesOfflineAndBroadcasts(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.Node{
		ID:       ulid.Make().String(),
		Name:     "stale-node",
		LastSeen: time.Now().UTC().Add(-10 * time.Minute),
		Status:   "online",
	}).Error)

	eventHub := hub.New()
	_, ch, _, unsub, err := eventHub.Subscribe("user-1", "127.0.0.1")
	require.NoError(t, err)
	defer unsub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handlers.StartNodeStatusMonitor(ctx, database, eventHub, 10*time.Millisecond)

	var message handlers.WSMessage
	select {
	case raw := <-ch:
		require.NoError(t, json.Unmarshal(raw, &message))
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for node_status broadcast")
	}

	assert.Equal(t, "node_status", message.Type)

	require.Eventually(t, func() bool {
		var node models.Node
		if err := database.Where("name = ?", "stale-node").First(&node).Error; err != nil {
			return false
		}
		return node.Status == "offline"
	}, time.Second, 20*time.Millisecond)
}
