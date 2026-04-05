package incidents_test

import (
	"testing"
	"time"

	"blackbox/server/internal/db"
	"blackbox/server/internal/hub"
	"blackbox/server/internal/incidents"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeEntry(service, source, event, metadata string) types.Entry {
	return types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "node-01",
		Service:   service,
		Source:    source,
		Event:     event,
		Metadata:  metadata,
	}
}

func TestConfirmedIncident_OpenOnDown(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := incidents.NewManager(database, hub.New())
	ch := incidents.NewChannel()
	go mgr.Run(t.Context(), ch)

	entry := makeEntry("nginx", "webhook", "down", `{"monitor":"nginx"}`)
	require.NoError(t, database.Create(&entry).Error)
	ch <- entry
	time.Sleep(100 * time.Millisecond)

	var incident models.Incident
	err = database.Where("status = ? AND confidence = ?", "open", "confirmed").First(&incident).Error
	require.NoError(t, err)
	assert.Equal(t, "open", incident.Status)
	assert.Equal(t, "confirmed", incident.Confidence)
	assert.Equal(t, entry.ID, incident.TriggerID)
}

func TestConfirmedIncident_ResolveOnUp(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := incidents.NewManager(database, hub.New())
	ch := incidents.NewChannel()
	go mgr.Run(t.Context(), ch)

	downEntry := makeEntry("nginx", "webhook", "down", `{"monitor":"nginx"}`)
	require.NoError(t, database.Create(&downEntry).Error)
	ch <- downEntry
	time.Sleep(100 * time.Millisecond)

	upEntry := makeEntry("nginx", "webhook", "up", `{"monitor":"nginx"}`)
	require.NoError(t, database.Create(&upEntry).Error)
	ch <- upEntry
	time.Sleep(100 * time.Millisecond)

	var incident models.Incident
	require.NoError(t, database.First(&incident).Error)
	assert.Equal(t, "resolved", incident.Status)
	assert.NotNil(t, incident.ResolvedAt)
}
