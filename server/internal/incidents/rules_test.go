package incidents

import (
	"context"
	"testing"
	"time"

	"blackbox/server/internal/db"
	"blackbox/server/internal/hub"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_ReplayCutoff_SkipsOldEntries(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)
	defer func() {
		sqlDB, _ := database.DB()
		sqlDB.Close()
	}()

	t.Setenv("INCIDENT_REPLAY_CUTOFF", "5m")

	mgr := NewManager(database, nil, nil)
	ch := NewChannel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mgr.Run(ctx, ch)

	old := types.Entry{
		ID:        "01OLDENTRY0000000",
		NodeName:  "node1",
		Source:    "webhook",
		Event:     "down",
		Service:   "myapp",
		Timestamp: time.Now().Add(-10 * time.Minute),
	}
	ch <- old

	time.Sleep(100 * time.Millisecond)

	var count int64
	require.NoError(t, database.Model(&models.Incident{}).Count(&count).Error)
	assert.Equal(t, int64(0), count, "old replayed entry should not create an incident")
}

func makeEntry(service, source, event, metadata string) types.Entry {
	return makeEntryAt(service, source, event, metadata, time.Now().UTC())
}

func makeEntryAt(service, source, event, metadata string, ts time.Time) types.Entry {
	return types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: ts,
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

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	entry := makeEntry("nginx", "webhook", "down", `{"monitor":"nginx"}`)
	require.NoError(t, database.Create(&entry).Error)
	ch <- entry

	var incident models.Incident
	require.Eventually(t, func() bool {
		err = database.Where("status = ? AND confidence = ?", "open", "confirmed").First(&incident).Error
		return err == nil
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, "open", incident.Status)
	assert.Equal(t, "confirmed", incident.Confidence)
	assert.Equal(t, entry.ID, incident.TriggerID)
}

func TestConfirmedIncident_OpenOnDown_UsesCorrelatedCauseNodeNames(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	causeEntry := makeEntryAt("radarr", "docker", "stop", `{"log_snippet":["fatal: config broke"]}`, now.Add(-30*time.Second))
	causeEntry.NodeName = "media-node"
	require.NoError(t, database.Create(&causeEntry).Error)

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	downEntry := makeEntryAt("radarr", "webhook", "down", `{"monitor":"radarr"}`, now)
	downEntry.NodeName = "webhook"
	require.NoError(t, database.Create(&downEntry).Error)
	ch <- downEntry

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.Where("status = ? AND confidence = ?", "open", "confirmed").First(&incident).Error; err != nil {
			return false
		}
		return incident.TriggerID == downEntry.ID
	}, time.Second, 10*time.Millisecond)

	assert.Equal(t, `["media-node"]`, incident.NodeNames)
	assert.Equal(t, causeEntry.ID, incident.RootCauseID)
}

func TestConfirmedIncident_OpenOnDown_BindsFollowUpNodeEventsToScopedIncident(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	causeEntry := makeEntryAt("radarr", "docker", "stop", `{}`, now.Add(-30*time.Second))
	causeEntry.NodeName = "media-node"
	require.NoError(t, database.Create(&causeEntry).Error)

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	downEntry := makeEntryAt("radarr", "webhook", "down", `{"monitor":"radarr"}`, now)
	downEntry.NodeName = "webhook"
	require.NoError(t, database.Create(&downEntry).Error)
	ch <- downEntry

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.Where("status = ? AND confidence = ?", "open", "confirmed").First(&incident).Error; err != nil {
			return false
		}
		return incident.TriggerID == downEntry.ID
	}, time.Second, 10*time.Millisecond)

	followup := makeEntryAt("radarr", "docker", "die", `{"exitCode":137}`, now.Add(10*time.Second))
	followup.NodeName = "media-node"
	require.NoError(t, database.Create(&followup).Error)
	ch <- followup

	require.Eventually(t, func() bool {
		var link models.IncidentEntry
		if err := database.Where("incident_id = ? AND entry_id = ?", incident.ID, followup.ID).First(&link).Error; err != nil {
			return false
		}
		return link.Role == "evidence"
	}, time.Second, 10*time.Millisecond)
}

func TestConfirmedIncident_ResolveOnUp(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	downEntry := makeEntry("nginx", "webhook", "down", `{"monitor":"nginx"}`)
	require.NoError(t, database.Create(&downEntry).Error)
	ch <- downEntry

	upEntry := makeEntry("nginx", "webhook", "up", `{"monitor":"nginx"}`)
	require.NoError(t, database.Create(&upEntry).Error)
	ch <- upEntry

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.First(&incident).Error; err != nil {
			return false
		}
		return incident.Status == "resolved" && incident.ResolvedAt != nil
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, "resolved", incident.Status)
	assert.NotNil(t, incident.ResolvedAt)
}

func TestConfirmedIncident_DockerStartAddsEvidenceButDoesNotResolve(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	causeEntry := makeEntryAt("radarr", "docker", "stop", `{}`, now.Add(-30*time.Second))
	causeEntry.NodeName = "media-node"
	require.NoError(t, database.Create(&causeEntry).Error)

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	downEntry := makeEntryAt("radarr", "webhook", "down", `{"monitor":"radarr"}`, now)
	downEntry.NodeName = "webhook"
	require.NoError(t, database.Create(&downEntry).Error)
	ch <- downEntry

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.Where("status = ? AND confidence = ?", "open", "confirmed").First(&incident).Error; err != nil {
			return false
		}
		return incident.TriggerID == downEntry.ID
	}, time.Second, 10*time.Millisecond)

	startEntry := makeEntryAt("radarr", "docker", "start", `{}`, now.Add(15*time.Second))
	startEntry.NodeName = "media-node"
	require.NoError(t, database.Create(&startEntry).Error)
	ch <- startEntry

	require.Eventually(t, func() bool {
		var link models.IncidentEntry
		if err := database.Where("incident_id = ? AND entry_id = ?", incident.ID, startEntry.ID).First(&link).Error; err != nil {
			return false
		}
		return link.Role == "evidence"
	}, time.Second, 10*time.Millisecond)

	require.Never(t, func() bool {
		if err := database.First(&incident, "id = ?", incident.ID).Error; err != nil {
			return false
		}
		return incident.Status == "resolved"
	}, 150*time.Millisecond, 25*time.Millisecond)
}

func TestSuspectedIncident_DockerExitDuringRecoveryWindow_KeepsIncidentOpen(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	dieEntry := makeEntryAt("radarr", "docker", "die", `{"exitCode":137}`, now)
	dieEntry.NodeName = "media-node"
	require.NoError(t, database.Create(&dieEntry).Error)
	ch <- dieEntry

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.Where("status = ? AND confidence = ?", "open", "suspected").First(&incident).Error; err != nil {
			return false
		}
		return incident.TriggerID == dieEntry.ID
	}, time.Second, 10*time.Millisecond)

	startEntry := makeEntryAt("radarr", "docker", "start", `{}`, now.Add(15*time.Second))
	startEntry.NodeName = "media-node"
	require.NoError(t, database.Create(&startEntry).Error)
	ch <- startEntry

	stopEntry := makeEntryAt("radarr", "docker", "stop", `{"exitCode":1}`, startEntry.Timestamp.Add(10*time.Second))
	stopEntry.NodeName = "media-node"
	require.NoError(t, database.Create(&stopEntry).Error)
	ch <- stopEntry

	mgr.mu.Lock()
	sweepExpiredRecoveriesLocked(mgr, startEntry.Timestamp.Add(dockerStabilityTTL+time.Second))
	mgr.mu.Unlock()

	require.NoError(t, database.First(&incident, "id = ?", incident.ID).Error)
	assert.Equal(t, "open", incident.Status)
	assert.Nil(t, incident.ResolvedAt)
}

func TestConfirmedIncident_ResolveOnUpAfterManagerRestart(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	incident := models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   now.Add(-5 * time.Minute),
		Status:     "open",
		Confidence: "confirmed",
		Title:      "nginx - monitor down",
		Services:   `["nginx"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   "{}",
	}
	require.NoError(t, database.Create(&incident).Error)

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	upEntry := makeEntryAt("nginx", "webhook", "up", `{"monitor":"nginx"}`, now)
	upEntry.NodeName = "webhook"
	require.NoError(t, database.Create(&upEntry).Error)
	ch <- upEntry

	require.Eventually(t, func() bool {
		if err := database.First(&incident, "id = ?", incident.ID).Error; err != nil {
			return false
		}
		return incident.Status == "resolved" && incident.ResolvedAt != nil
	}, time.Second, 10*time.Millisecond)
}

func TestSuspectedIncident_OpensOnNumericExitCode(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	entry := makeEntry("nginx", "docker", "die", `{"exitCode":137}`)
	require.NoError(t, database.Create(&entry).Error)
	ch <- entry

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.Where("confidence = ?", "suspected").First(&incident).Error; err != nil {
			return false
		}
		return incident.TriggerID == entry.ID
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, "suspected", incident.Confidence)
	assert.Equal(t, "nginx — container exited (exit 137)", incident.Title)

	var links []models.IncidentEntry
	require.NoError(t, database.Where("incident_id = ?", incident.ID).Find(&links).Error)
	require.Len(t, links, 1)
	assert.Equal(t, entry.ID, links[0].EntryID)
	assert.Equal(t, "trigger", links[0].Role)
	assert.Empty(t, incident.RootCauseID)
}

func TestSuspectedIncident_CrashLoopWithZeroExit_UsesRepeatedExitWording(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := NewManager(database, hub.New(), nil)
	now := time.Now().UTC()

	for i := 0; i < crashLoopThreshold; i++ {
		entry := makeEntryAt("nginx", "docker", "die", `{"exitCode":0}`, now.Add(time.Duration(i)*time.Second))
		require.NoError(t, database.Create(&entry).Error)
		mgr.processEntry(entry)
	}

	var incident models.Incident
	require.NoError(t, database.Where("confidence = ?", "suspected").First(&incident).Error)
	assert.Equal(t, "nginx — container exited repeatedly", incident.Title)
}

func TestConfirmedIncident_UpgradeSkipsAlreadyLinkedCauseEntries(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	crashEntry := makeEntry("radarr", "docker", "die", `{"exitCode":137}`)
	require.NoError(t, database.Create(&crashEntry).Error)
	ch <- crashEntry

	var suspected models.Incident
	require.Eventually(t, func() bool {
		if err := database.Where("confidence = ?", "suspected").First(&suspected).Error; err != nil {
			return false
		}
		return suspected.TriggerID == crashEntry.ID
	}, time.Second, 10*time.Millisecond)

	downEntry := makeEntry("radarr", "webhook", "down", `{"monitor":"radarr"}`)
	require.NoError(t, database.Create(&downEntry).Error)
	ch <- downEntry

	var upgraded models.Incident
	require.Eventually(t, func() bool {
		if err := database.First(&upgraded, "id = ?", suspected.ID).Error; err != nil {
			return false
		}
		return upgraded.Confidence == "confirmed" && upgraded.TriggerID == downEntry.ID
	}, time.Second, 10*time.Millisecond)

	var links []models.IncidentEntry
	require.NoError(t, database.Where("incident_id = ?", upgraded.ID).Order("entry_id ASC").Find(&links).Error)
	require.Len(t, links, 2)
	assert.Equal(t, crashEntry.ID, links[0].EntryID)
	assert.Equal(t, "evidence", links[0].Role)
	assert.Equal(t, downEntry.ID, links[1].EntryID)
	assert.Equal(t, "trigger", links[1].Role)
}

func TestSystemdFailed_OpensSuspectedIncident(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	entry := makeEntry("nginx.service", "systemd", "failed", `{}`)
	require.NoError(t, database.Create(&entry).Error)
	ch <- entry

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.Where("confidence = ?", "suspected").First(&incident).Error; err != nil {
			return false
		}
		return incident.TriggerID == entry.ID
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, "nginx.service — systemd unit failed", incident.Title)
}

func TestSystemdOOMKill_OpensSuspectedIncident(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	entry := makeEntry("kernel", "systemd", "oom_kill", `{"killed_process":"nginx"}`)
	require.NoError(t, database.Create(&entry).Error)
	ch <- entry

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.Where("confidence = ?", "suspected").First(&incident).Error; err != nil {
			return false
		}
		return incident.TriggerID == entry.ID
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, "kernel — systemd OOM kill", incident.Title)
}

func TestSystemdRestartLoop_OpensSuspectedIncident(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := NewManager(database, hub.New(), nil)
	now := time.Now().UTC()

	for i := 0; i < crashLoopThreshold; i++ {
		entry := makeEntryAt("nginx.service", "systemd", "restart", `{}`, now.Add(time.Duration(i)*time.Second))
		require.NoError(t, database.Create(&entry).Error)
		mgr.processEntry(entry)
	}

	var incident models.Incident
	require.NoError(t, database.Where("confidence = ?", "suspected").First(&incident).Error)
	assert.Equal(t, "nginx.service — systemd restart/failure loop", incident.Title)
}

func TestSystemdLoneRestart_DoesNotOpenIncident(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := NewManager(database, hub.New(), nil)
	entry := makeEntry("nginx.service", "systemd", "restart", `{}`)
	require.NoError(t, database.Create(&entry).Error)

	mgr.processEntry(entry)

	var count int64
	require.NoError(t, database.Model(&models.Incident{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSystemdStarted_ResolvesAfterStabilityWindow(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := NewManager(database, hub.New(), nil)
	base := time.Now().UTC()

	failed := makeEntryAt("nginx.service", "systemd", "failed", `{}`, base)
	require.NoError(t, database.Create(&failed).Error)
	mgr.processEntry(failed)

	started := makeEntryAt("nginx.service", "systemd", "started", `{}`, base.Add(time.Second))
	require.NoError(t, database.Create(&started).Error)
	mgr.processEntry(started)

	sweepExpiredRecoveriesLocked(mgr, started.Timestamp.Add(systemdStabilityTTL+time.Second))

	var incident models.Incident
	require.NoError(t, database.First(&incident).Error)
	require.Equal(t, "resolved", incident.Status)
	require.NotNil(t, incident.ResolvedAt)

	var recovery models.IncidentEntry
	require.NoError(t, database.Where("incident_id = ? AND role = ?", incident.ID, "recovery").First(&recovery).Error)
	assert.Equal(t, started.ID, recovery.EntryID)
}

func TestConfirmedSystemdStartedAddsEvidenceButDoesNotResolve(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	now := time.Now().UTC()
	causeEntry := makeEntryAt("nginx.service", "systemd", "failed", `{}`, now.Add(-30*time.Second))
	causeEntry.NodeName = "node-01"
	require.NoError(t, database.Create(&causeEntry).Error)

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	downEntry := makeEntryAt("nginx.service", "webhook", "down", `{"monitor":"nginx.service"}`, now)
	downEntry.NodeName = "webhook"
	require.NoError(t, database.Create(&downEntry).Error)
	ch <- downEntry

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.Where("status = ? AND confidence = ?", "open", "confirmed").First(&incident).Error; err != nil {
			return false
		}
		return incident.TriggerID == downEntry.ID
	}, time.Second, 10*time.Millisecond)

	started := makeEntryAt("nginx.service", "systemd", "started", `{}`, now.Add(time.Second))
	started.NodeName = "node-01"
	require.NoError(t, database.Create(&started).Error)
	ch <- started

	require.Eventually(t, func() bool {
		var link models.IncidentEntry
		if err := database.Where("incident_id = ? AND entry_id = ?", incident.ID, started.ID).First(&link).Error; err != nil {
			return false
		}
		return link.Role == "evidence"
	}, time.Second, 10*time.Millisecond)

	require.Never(t, func() bool {
		if err := database.First(&incident, "id = ?", incident.ID).Error; err != nil {
			return false
		}
		return incident.Status == "resolved"
	}, 150*time.Millisecond, 25*time.Millisecond)
}

func TestSystemdFailureDuringRecoveryWindow_KeepsIncidentOpen(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := NewManager(database, hub.New(), nil)
	base := time.Now().UTC()

	failed := makeEntryAt("nginx.service", "systemd", "failed", `{}`, base)
	require.NoError(t, database.Create(&failed).Error)
	mgr.processEntry(failed)

	started := makeEntryAt("nginx.service", "systemd", "started", `{}`, base.Add(time.Second))
	require.NoError(t, database.Create(&started).Error)
	mgr.processEntry(started)

	restarted := makeEntryAt("nginx.service", "systemd", "restart", `{}`, base.Add(30*time.Second))
	require.NoError(t, database.Create(&restarted).Error)
	mgr.processEntry(restarted)

	sweepExpiredRecoveriesLocked(mgr, started.Timestamp.Add(systemdStabilityTTL+time.Second))

	var incident models.Incident
	require.NoError(t, database.First(&incident).Error)
	assert.Equal(t, "open", incident.Status)
	assert.Nil(t, incident.ResolvedAt)
}

func TestWatchtowerUpdateFollowedByRestart_OpensSuspectedIncident(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	mgr := NewManager(database, hub.New(), nil)
	ch := NewChannel()
	go mgr.Run(t.Context(), ch)

	base := time.Now().UTC()
	update := makeEntryAt("watchtower", "webhook", "update", `{"watchtower.services":["sonarr"]}`, base)
	update.NodeName = "webhook"
	require.NoError(t, database.Create(&update).Error)
	ch <- update

	restart := makeEntryAt("sonarr", "docker", "start", `{}`, base.Add(15*time.Second))
	restart.NodeName = "media-node"
	require.NoError(t, database.Create(&restart).Error)
	ch <- restart

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.Where("confidence = ?", "suspected").First(&incident).Error; err != nil {
			return false
		}
		return incident.TriggerID == restart.ID && incident.RootCauseID == update.ID
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, `["sonarr"]`, incident.Services)

	var cause models.IncidentEntry
	require.Eventually(t, func() bool {
		return database.Where("incident_id = ? AND role = ?", incident.ID, "cause").First(&cause).Error == nil
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, update.ID, cause.EntryID)
}

func TestWatchtowerTargetServices_NormalizesCase(t *testing.T) {
	entry := types.Entry{
		Service:  "WatchTower",
		Metadata: `{"watchtower.services":[" Sonarr ","sonarr","SONARR"]}`,
	}

	assert.Equal(t, []string{"sonarr"}, watchtowerTargetServices(entry))
}
