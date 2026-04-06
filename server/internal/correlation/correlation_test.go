package correlation_test

import (
	"testing"
	"time"

	"blackbox/server/internal/correlation"
	"blackbox/server/internal/db"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
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

func TestScoreCauses_SystemdFailedScores90(t *testing.T) {
	database := newTestDB(t)
	now := time.Now().UTC()

	failedID := ulid.Make().String()
	require.NoError(t, database.Create(&types.Entry{
		ID:        failedID,
		Timestamp: now.Add(-60 * time.Second),
		NodeName:  "node-01",
		Source:    "systemd",
		Service:   "nginx.service",
		Event:     "failed",
		Content:   "nginx.service failed",
		Metadata:  `{}`,
	}).Error)

	candidates, err := correlation.ScoreCauses(database, []string{"nginx.service"}, now)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, failedID, candidates[0].Entry.ID)
	require.GreaterOrEqual(t, candidates[0].Score, 90)
}

func TestScoreCauses_OOMKillScores100(t *testing.T) {
	database := newTestDB(t)
	now := time.Now().UTC()

	oomID := ulid.Make().String()
	require.NoError(t, database.Create(&types.Entry{
		ID:        oomID,
		Timestamp: now.Add(-30 * time.Second),
		NodeName:  "node-01",
		Source:    "systemd",
		Service:   "kernel",
		Event:     "oom_kill",
		Content:   "OOM kill: nginx",
		Metadata:  `{}`,
	}).Error)

	candidates, err := correlation.ScoreCauses(database, []string{"kernel"}, now)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, oomID, candidates[0].Entry.ID)
	require.Equal(t, 100, candidates[0].Score)
}
