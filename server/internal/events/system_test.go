package events_test

import (
	"testing"
	"time"

	"blackbox/server/internal/db"
	"blackbox/server/internal/events"
	"blackbox/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogSystem_WritesEntryToTimeline(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	events.LogSystem(database, "auth", "user.login", "user alice logged in")

	var entries []types.Entry
	require.NoError(t, database.Find(&entries).Error)
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, "system", e.Source)
	assert.Equal(t, "server", e.NodeName)
	assert.Equal(t, "auth", e.Service)
	assert.Equal(t, "user.login", e.Event)
	assert.Equal(t, "user alice logged in", e.Content)
	assert.NotEmpty(t, e.ID)
	assert.WithinDuration(t, time.Now().UTC(), e.Timestamp, 5*time.Second)
}

func TestLogSystem_DoesNotPanicOnDBError(t *testing.T) {
	assert.NotPanics(t, func() {
		events.LogSystem(nil, "auth", "user.login", "should not panic")
	})
}
