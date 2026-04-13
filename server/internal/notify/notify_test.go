package notify

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"blackbox/server/internal/db"
	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDispatcher_Send_RoutesToEnabledDest(t *testing.T) {
	database := newTestDB(t)
	events, err := json.Marshal([]string{EventIncidentOpenedConfirmed})
	require.NoError(t, err)
	require.NoError(t, database.Create(&models.NotificationDest{
		ID:      "dest-1",
		Name:    "Test Discord",
		Type:    "discord",
		URL:     "https://example.invalid/discord",
		Events:  string(events),
		Enabled: true,
	}).Error)

	var hits atomic.Int32
	restoreDiscordSender(t, func(ctx context.Context, webhookURL string, inc models.Incident, test bool) error {
		hits.Add(1)
		return nil
	})

	d := NewDispatcher(database)
	d.Send(context.Background(), EventIncidentOpenedConfirmed, testIncident())

	require.Eventually(t, func() bool {
		return hits.Load() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestDispatcher_Send_SkipsDisabledDest(t *testing.T) {
	database := newTestDB(t)
	events, err := json.Marshal([]string{EventIncidentOpenedConfirmed})
	require.NoError(t, err)
	require.NoError(t, database.Create(&models.NotificationDest{
		ID:      "dest-2",
		Name:    "Disabled",
		Type:    "discord",
		URL:     "https://example.invalid/discord",
		Events:  string(events),
		Enabled: false,
	}).Error)

	var hits atomic.Int32
	restoreDiscordSender(t, func(ctx context.Context, webhookURL string, inc models.Incident, test bool) error {
		hits.Add(1)
		return nil
	})

	d := NewDispatcher(database)
	d.Send(context.Background(), EventIncidentOpenedConfirmed, testIncident())

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), hits.Load())
}

func TestDispatcher_Send_SkipsNonMatchingEvent(t *testing.T) {
	database := newTestDB(t)
	events, err := json.Marshal([]string{EventIncidentResolved})
	require.NoError(t, err)
	require.NoError(t, database.Create(&models.NotificationDest{
		ID:      "dest-3",
		Name:    "Resolved only",
		Type:    "discord",
		URL:     "https://example.invalid/discord",
		Events:  string(events),
		Enabled: true,
	}).Error)

	var hits atomic.Int32
	restoreDiscordSender(t, func(ctx context.Context, webhookURL string, inc models.Incident, test bool) error {
		hits.Add(1)
		return nil
	})

	d := NewDispatcher(database)
	d.Send(context.Background(), EventIncidentOpenedConfirmed, testIncident())

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), hits.Load())
}

func TestDispatcher_SendTest_ReturnsSenderError(t *testing.T) {
	database := newTestDB(t)
	expectedErr := errors.New("provider failed")
	restoreDiscordSender(t, func(ctx context.Context, webhookURL string, inc models.Incident, test bool) error {
		return expectedErr
	})

	d := NewDispatcher(database)
	err := d.SendTest(context.Background(), models.NotificationDest{
		ID:      "dest-test",
		Name:    "Bad",
		Type:    "discord",
		URL:     "https://example.invalid/discord",
		Events:  `["incident_opened_confirmed"]`,
		Enabled: true,
	})

	assert.ErrorIs(t, err, expectedErr)
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := db.Init(":memory:")
	require.NoError(t, err)

	t.Cleanup(func() {
		sqlDB, err := database.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})

	return database
}

func restoreDiscordSender(t *testing.T, fn func(context.Context, string, models.Incident, bool) error) {
	t.Helper()

	previous := discordSender
	discordSender = fn
	t.Cleanup(func() {
		discordSender = previous
	})
}
