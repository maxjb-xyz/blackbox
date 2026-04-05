package correlation_test

import (
	"testing"
	"time"

	"blackbox/server/internal/correlation"
	"blackbox/server/internal/db"
	"blackbox/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := db.Init(":memory:")
	require.NoError(t, err)
	return database
}

func TestFindCause_ReturnsNilWhenNoEvents(t *testing.T) {
	database := newTestDB(t)
	cause, err := correlation.FindCause(database, "my-app", time.Now())
	require.NoError(t, err)
	assert.Nil(t, cause)
}

func TestFindCause_ReturnsMatchInWindow(t *testing.T) {
	database := newTestDB(t)
	now := time.Now().UTC()

	entry := types.Entry{
		ID:        "01TEST00000000001",
		Timestamp: now.Add(-60 * time.Second),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "my-app",
		Event:     "die",
		Content:   "container 'my-app' died (exit code: 137)",
	}
	require.NoError(t, database.Create(&entry).Error)

	cause, err := correlation.FindCause(database, "my-app", now)
	require.NoError(t, err)
	require.NotNil(t, cause)
	assert.Equal(t, "01TEST00000000001", cause.ID)
	assert.Equal(t, "homelab-01", cause.NodeName)
}

func TestFindCause_IgnoresEventsOutsideWindow(t *testing.T) {
	database := newTestDB(t)
	now := time.Now().UTC()

	entry := types.Entry{
		ID:        "01TEST00000000002",
		Timestamp: now.Add(-200 * time.Second),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "my-app",
		Event:     "die",
		Content:   "container 'my-app' died",
	}
	require.NoError(t, database.Create(&entry).Error)

	cause, err := correlation.FindCause(database, "my-app", now)
	require.NoError(t, err)
	assert.Nil(t, cause)
}

func TestFindCause_IgnoresWebhookSource(t *testing.T) {
	database := newTestDB(t)
	now := time.Now().UTC()

	entry := types.Entry{
		ID:        "01TEST00000000003",
		Timestamp: now.Add(-30 * time.Second),
		NodeName:  "webhook",
		Source:    "webhook",
		Service:   "my-app",
		Event:     "down",
		Content:   "Monitor 'my-app' is down",
	}
	require.NoError(t, database.Create(&entry).Error)

	cause, err := correlation.FindCause(database, "my-app", now)
	require.NoError(t, err)
	assert.Nil(t, cause)
}

func TestFindCause_ExactServiceMatchOnly(t *testing.T) {
	database := newTestDB(t)
	now := time.Now().UTC()

	entry := types.Entry{
		ID:        "01TEST00000000004",
		Timestamp: now.Add(-30 * time.Second),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "my-app-v2",
		Event:     "die",
		Content:   "container 'my-app-v2' died",
	}
	require.NoError(t, database.Create(&entry).Error)

	cause, err := correlation.FindCause(database, "my-app", now)
	require.NoError(t, err)
	assert.Nil(t, cause)
}

func TestFindCause_ReturnsHighestScoringCandidateWhenMultiple(t *testing.T) {
	database := newTestDB(t)
	now := time.Now().UTC()

	older := types.Entry{
		ID:        "01TEST00000000005",
		Timestamp: now.Add(-100 * time.Second),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "my-app",
		Event:     "stop",
		Content:   "container stopped",
	}
	newer := types.Entry{
		ID:        "01TEST00000000006",
		Timestamp: now.Add(-30 * time.Second),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "my-app",
		Event:     "die",
		Content:   "container died",
	}
	require.NoError(t, database.Create(&older).Error)
	require.NoError(t, database.Create(&newer).Error)

	cause, err := correlation.FindCause(database, "my-app", now)
	require.NoError(t, err)
	require.NotNil(t, cause)
	assert.Equal(t, "01TEST00000000005", cause.ID)
}
