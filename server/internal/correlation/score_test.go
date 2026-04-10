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
)

func TestScoreCauses_DieNonZeroExit(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	cause := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: now.Add(-10 * time.Second),
		Service:   "nginx",
		Source:    "docker",
		Event:     "die",
		Metadata:  `{"exitCode":"1"}`,
	}
	require.NoError(t, database.Create(&cause).Error)

	candidates, err := correlation.ScoreCauses(database, []string{"nginx"}, now, "")
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, cause.ID, candidates[0].Entry.ID)
	assert.Equal(t, 93, candidates[0].Score)
}

func TestScoreCauses_DieNonZeroNumericExit(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	cause := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: now.Add(-10 * time.Second),
		Service:   "nginx",
		Source:    "docker",
		Event:     "die",
		Metadata:  `{"exitCode":137}`,
	}
	require.NoError(t, database.Create(&cause).Error)

	candidates, err := correlation.ScoreCauses(database, []string{"nginx"}, now, "")
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, 93, candidates[0].Score)
}

func TestScoreCauses_ExcludesWebhooks(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	webhook := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: now.Add(-5 * time.Second),
		Service:   "nginx",
		Source:    "webhook",
		Event:     "down",
		Metadata:  `{}`,
	}
	require.NoError(t, database.Create(&webhook).Error)

	candidates, err := correlation.ScoreCauses(database, []string{"nginx"}, now, "")
	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestScoreCauses_ExcludesOutsideWindow(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	old := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: now.Add(-400 * time.Second),
		Service:   "nginx",
		Source:    "docker",
		Event:     "die",
		Metadata:  `{"exitCode":"1"}`,
	}
	require.NoError(t, database.Create(&old).Error)

	candidates, err := correlation.ScoreCauses(database, []string{"nginx"}, now, "")
	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestScoreCauses_RankedByScore(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	restart := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: now.Add(-20 * time.Second),
		Service:   "nginx",
		Source:    "docker",
		Event:     "restart",
		Metadata:  `{}`,
	}
	fileWrite := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: now.Add(-30 * time.Second),
		Service:   "nginx",
		Source:    "files",
		Event:     "write",
		Metadata:  `{}`,
	}
	require.NoError(t, database.Create(&restart).Error)
	require.NoError(t, database.Create(&fileWrite).Error)

	candidates, err := correlation.ScoreCauses(database, []string{"nginx"}, now, "")
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, restart.ID, candidates[0].Entry.ID) // decay still leaves restart ahead of file write
	assert.Equal(t, fileWrite.ID, candidates[1].Entry.ID)
}

func TestScoreCauses_TimeDecayReducesScoreAtWindowEdge(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	near := types.Entry{
		ID:        "01NEARDIE",
		Timestamp: now.Add(-5 * time.Second),
		Service:   "nginx",
		Source:    "docker",
		Event:     "die",
		Metadata:  `{"exitCode":"1"}`,
	}
	far := types.Entry{
		ID:        "01FARDIE0",
		Timestamp: now.Add(-55 * time.Second),
		Service:   "nginx",
		Source:    "docker",
		Event:     "die",
		Metadata:  `{"exitCode":"1"}`,
	}
	require.NoError(t, database.Create(&near).Error)
	require.NoError(t, database.Create(&far).Error)

	candidates, err := correlation.ScoreCauses(database, []string{"nginx"}, now, "")
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, near.ID, candidates[0].Entry.ID)
	assert.Greater(t, candidates[0].Score, candidates[1].Score)
}

func TestScoreCauses_ComposeServiceFiltersDockerCandidates(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	redis := types.Entry{
		ID:             "01REDISDIE",
		Timestamp:      now.Add(-10 * time.Second),
		Service:        "myapp",
		Source:         "docker",
		ComposeService: "redis",
		Event:          "die",
		Metadata:       `{"exitCode":"1"}`,
	}
	web := types.Entry{
		ID:             "01WEBDOCKR",
		Timestamp:      now.Add(-12 * time.Second),
		Service:        "myapp",
		Source:         "docker",
		ComposeService: "web",
		Event:          "die",
		Metadata:       `{"exitCode":"1"}`,
	}
	systemd := types.Entry{
		ID:        "01SYSTEMD1",
		Timestamp: now.Add(-8 * time.Second),
		Service:   "myapp",
		Source:    "systemd",
		Event:     "failed",
		Metadata:  `{}`,
	}
	require.NoError(t, database.Create(&redis).Error)
	require.NoError(t, database.Create(&web).Error)
	require.NoError(t, database.Create(&systemd).Error)

	candidates, err := correlation.ScoreCauses(database, []string{"myapp"}, now, "web")
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	returnedIDs := []string{candidates[0].Entry.ID, candidates[1].Entry.ID}
	assert.Contains(t, returnedIDs, web.ID)
	assert.Contains(t, returnedIDs, systemd.ID)
	assert.NotContains(t, returnedIDs, redis.ID)
}

func TestScoreCauses_EmptyTriggerComposeServiceAllowsAllDockerCandidates(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	redis := types.Entry{
		ID:             "01REDISALL",
		Timestamp:      now.Add(-10 * time.Second),
		Service:        "myapp",
		Source:         "docker",
		ComposeService: "redis",
		Event:          "die",
		Metadata:       `{"exitCode":"1"}`,
	}
	web := types.Entry{
		ID:             "01WEBALLOW",
		Timestamp:      now.Add(-12 * time.Second),
		Service:        "myapp",
		Source:         "docker",
		ComposeService: "web",
		Event:          "die",
		Metadata:       `{"exitCode":"1"}`,
	}
	require.NoError(t, database.Create(&redis).Error)
	require.NoError(t, database.Create(&web).Error)

	candidates, err := correlation.ScoreCauses(database, []string{"myapp"}, now, "")
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, redis.ID, candidates[0].Entry.ID)
	assert.Equal(t, web.ID, candidates[1].Entry.ID)
}

func TestApplyNodeBonus(t *testing.T) {
	entry := &types.Entry{NodeName: "node-01"}
	candidates := []correlation.CauseCandidate{
		{Entry: entry, Score: 80},
	}

	correlation.ApplyNodeBonus(candidates, "node-01")

	assert.Equal(t, 100, candidates[0].Score)
}

func TestScoreCauses_TieBreaksByTimestampThenID(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	older := types.Entry{
		ID:        "01OLDER",
		Timestamp: now.Add(-20 * time.Second),
		Service:   "nginx",
		Source:    "docker",
		Event:     "restart",
		Metadata:  `{}`,
	}
	newer := types.Entry{
		ID:        "01NEWER",
		Timestamp: now.Add(-10 * time.Second),
		Service:   "nginx",
		Source:    "docker",
		Event:     "restart",
		Metadata:  `{}`,
	}
	require.NoError(t, database.Create(&older).Error)
	require.NoError(t, database.Create(&newer).Error)

	candidates, err := correlation.ScoreCauses(database, []string{"nginx"}, now, "")
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, newer.ID, candidates[0].Entry.ID)
	assert.Equal(t, older.ID, candidates[1].Entry.ID)

	// Equal score and timestamp should fall back to ID ordering.
	candidates = []correlation.CauseCandidate{
		{Entry: &types.Entry{ID: "02B", Timestamp: now, NodeName: "node-01"}, Score: 80},
		{Entry: &types.Entry{ID: "02A", Timestamp: now}, Score: 80},
	}
	correlation.ApplyNodeBonus(candidates, "")
	assert.Equal(t, "02A", candidates[0].Entry.ID)
	assert.Equal(t, "02B", candidates[1].Entry.ID)
}
