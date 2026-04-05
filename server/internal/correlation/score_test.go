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

	candidates, err := correlation.ScoreCauses(database, []string{"nginx"}, now)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, cause.ID, candidates[0].Entry.ID)
	assert.Equal(t, 100, candidates[0].Score)
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

	candidates, err := correlation.ScoreCauses(database, []string{"nginx"}, now)
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

	candidates, err := correlation.ScoreCauses(database, []string{"nginx"}, now)
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

	candidates, err := correlation.ScoreCauses(database, []string{"nginx"}, now)
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, restart.ID, candidates[0].Entry.ID) // score 90 > 50
	assert.Equal(t, fileWrite.ID, candidates[1].Entry.ID)
}

func TestApplyNodeBonus(t *testing.T) {
	entry := &types.Entry{NodeName: "node-01"}
	candidates := []correlation.CauseCandidate{
		{Entry: entry, Score: 80},
	}

	correlation.ApplyNodeBonus(candidates, "node-01")

	assert.Equal(t, 100, candidates[0].Score)
}
