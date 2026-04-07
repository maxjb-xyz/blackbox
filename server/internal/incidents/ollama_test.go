package incidents

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"blackbox/server/internal/db"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func TestManager_SuspectedIncidentDispatchesAIAnalysisWithTriggerLogs(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaModelKey, Value: "llama3.2"}).Error)

	promptSeen := make(chan string, 1)
	originalCall := callOllamaFunc
	callOllamaFunc = func(baseURL, model, prompt string) (string, error) {
		promptSeen <- prompt
		return "Crash caused by invalid config", nil
	}
	defer func() { callOllamaFunc = originalCall }()

	now := time.Now().UTC()
	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: now,
		NodeName:  "node-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "die",
		Content:   "Container stopped: nginx (exit code: 137)",
		Metadata:  `{"exitCode":137,"log_snippet":["fatal: invalid nginx.conf"]}`,
	}
	require.NoError(t, database.Create(&entry).Error)

	manager := NewManager(database, nil)
	manager.processEntry(entry)

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.Where("confidence = ?", "suspected").First(&incident).Error; err != nil {
			return false
		}
		meta := parseIncidentTestMetadata(t, incident.Metadata)
		return meta["ai_analysis"] == "Crash caused by invalid config"
	}, time.Second, 10*time.Millisecond)

	select {
	case prompt := <-promptSeen:
		require.Contains(t, prompt, "[trigger] docker/die")
		require.Contains(t, prompt, "Log: fatal: invalid nginx.conf")
		require.True(t, strings.Contains(prompt, "Confidence: suspected"), prompt)
	default:
		t.Fatal("expected Ollama prompt for suspected incident")
	}
}

func TestOllamaEnricher_SetsAndClearsPendingState(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	release := make(chan struct{})
	originalCall := callOllamaFunc
	callOllamaFunc = func(baseURL, model, prompt string) (string, error) {
		<-release
		return "Root cause: bad config", nil
	}
	defer func() { callOllamaFunc = originalCall }()

	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaModelKey, Value: "llama3.2"}).Error)

	incidentID := ulid.Make().String()
	require.NoError(t, database.Create(&models.Incident{
		ID:         incidentID,
		OpenedAt:   time.Now().UTC(),
		Status:     "open",
		Confidence: "confirmed",
		Title:      "nginx down",
		Services:   `["nginx"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{}`,
	}).Error)

	enricher := NewOllamaEnricher(database, nil)
	enricher.EnrichAsync(incidentID, []enrichEntry{{
		Role:    "trigger",
		Content: "Container stopped: nginx",
		Source:  "docker",
		Event:   "stop",
	}})

	require.Eventually(t, func() bool {
		var inc models.Incident
		if err := database.First(&inc, "id = ?", incidentID).Error; err != nil {
			return false
		}
		meta := parseIncidentTestMetadata(t, inc.Metadata)
		pending, _ := meta["ai_pending"].(bool)
		return pending && meta["ai_model"] == "llama3.2"
	}, time.Second, 10*time.Millisecond)

	close(release)

	require.Eventually(t, func() bool {
		var inc models.Incident
		if err := database.First(&inc, "id = ?", incidentID).Error; err != nil {
			return false
		}
		meta := parseIncidentTestMetadata(t, inc.Metadata)
		_, pending := meta["ai_pending"]
		return !pending && meta["ai_analysis"] == "Root cause: bad config"
	}, time.Second, 10*time.Millisecond)
}

func TestCorrelateAsync_WritesAICauseLinks(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaModelKey, Value: "llama3.2"}).Error)

	now := time.Now().UTC()
	triggerID := ulid.Make().String()
	candidateID := ulid.Make().String()

	require.NoError(t, database.Create(&types.Entry{
		ID:        triggerID,
		Timestamp: now.Add(-2 * time.Minute),
		NodeName:  "node-01",
		Source:    "webhook",
		Service:   "nginx",
		Event:     "down",
		Content:   "nginx monitor down",
		Metadata:  `{}`,
	}).Error)
	require.NoError(t, database.Create(&types.Entry{
		ID:        candidateID,
		Timestamp: now.Add(-3 * time.Minute),
		NodeName:  "node-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "die",
		Content:   "container exited with OOMKilled",
		Metadata:  `{"log_snippet":["oom kill detected"]}`,
	}).Error)

	incidentID := ulid.Make().String()
	require.NoError(t, database.Create(&models.Incident{
		ID:         incidentID,
		OpenedAt:   now,
		Status:     "open",
		Confidence: "confirmed",
		Title:      "nginx down",
		Services:   `["nginx"]`,
		NodeNames:  `["node-01"]`,
		TriggerID:  triggerID,
		Metadata:   `{}`,
	}).Error)
	require.NoError(t, database.Create(&models.IncidentEntry{
		IncidentID: incidentID,
		EntryID:    triggerID,
		Role:       "trigger",
		Score:      0,
	}).Error)

	originalCall := callOllamaCorrelateFunc
	originalDelay := correlateDelay
	callOllamaCorrelateFunc = func(baseURL, model, prompt string) (string, error) {
		return `analysis wrapper {"summary":"nginx crashed due to resource exhaustion","causes":[{"entry_id":"` + candidateID + `","confidence":0.85,"reason":"Container exited before the outage"}]} done`, nil
	}
	correlateDelay = 0
	defer func() {
		callOllamaCorrelateFunc = originalCall
		correlateDelay = originalDelay
	}()

	enricher := NewOllamaEnricher(database, nil)
	enricher.CorrelateAsync(incidentID, nil, "node-01")

	require.Eventually(t, func() bool {
		var link models.IncidentEntry
		if err := database.Where("incident_id = ? AND entry_id = ?", incidentID, candidateID).First(&link).Error; err != nil {
			return false
		}
		return link.Role == "ai_cause" && link.Score == 85 && link.Reason == "Container exited before the outage"
	}, time.Second, 10*time.Millisecond)

	var inc models.Incident
	require.NoError(t, database.First(&inc, "id = ?", incidentID).Error)
	meta := parseIncidentTestMetadata(t, inc.Metadata)
	require.Equal(t, "nginx crashed due to resource exhaustion", meta["ai_analysis"])
}

func TestCorrelateAsync_UsesScopedIncidentNodesAndSetsVerified(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaModelKey, Value: "llama3.2"}).Error)

	now := time.Now().UTC()
	triggerID := ulid.Make().String()
	causeID := ulid.Make().String()
	timelineID := ulid.Make().String()

	require.NoError(t, database.Create(&types.Entry{
		ID:        triggerID,
		Timestamp: now,
		NodeName:  "webhook",
		Source:    "webhook",
		Service:   "radarr",
		Event:     "down",
		Content:   "radarr monitor down",
		Metadata:  `{}`,
	}).Error)
	require.NoError(t, database.Create(&types.Entry{
		ID:        causeID,
		Timestamp: now.Add(-30 * time.Second),
		NodeName:  "media-node",
		Source:    "docker",
		Service:   "radarr",
		Event:     "stop",
		Content:   "container stopped before outage",
		Metadata:  `{"log_snippet":["rss sync panic"]}`,
	}).Error)
	require.NoError(t, database.Create(&types.Entry{
		ID:        timelineID,
		Timestamp: now.Add(-15 * time.Second),
		NodeName:  "media-node",
		Source:    "systemd",
		Service:   "radarr.service",
		Event:     "failed",
		Content:   "radarr service failed",
		Metadata:  `{"log_snippet":["unit entered failed state"]}`,
	}).Error)

	incidentID := ulid.Make().String()
	require.NoError(t, database.Create(&models.Incident{
		ID:         incidentID,
		OpenedAt:   now,
		Status:     "open",
		Confidence: "confirmed",
		Title:      "radarr down",
		Services:   `["radarr"]`,
		NodeNames:  `["webhook"]`,
		TriggerID:  triggerID,
		Metadata:   `{}`,
	}).Error)
	require.NoError(t, database.Create(&models.IncidentEntry{
		IncidentID: incidentID,
		EntryID:    triggerID,
		Role:       "trigger",
		Score:      0,
	}).Error)
	require.NoError(t, database.Create(&models.IncidentEntry{
		IncidentID: incidentID,
		EntryID:    causeID,
		Role:       "cause",
		Score:      90,
	}).Error)

	promptSeen := make(chan string, 1)
	originalCall := callOllamaCorrelateFunc
	originalDelay := correlateDelay
	callOllamaCorrelateFunc = func(baseURL, model, prompt string) (string, error) {
		promptSeen <- prompt
		return `{"summary":"radarr failed on media-node","verified":true,"causes":[]}`, nil
	}
	correlateDelay = 0
	defer func() {
		callOllamaCorrelateFunc = originalCall
		correlateDelay = originalDelay
	}()

	enricher := NewOllamaEnricher(database, nil)
	enricher.CorrelateAsync(incidentID, nil, "webhook")

	var incident models.Incident
	require.Eventually(t, func() bool {
		if err := database.First(&incident, "id = ?", incidentID).Error; err != nil {
			return false
		}
		meta := parseIncidentTestMetadata(t, incident.Metadata)
		return meta["ai_analysis"] == "radarr failed on media-node" && meta["ai_verified"] == true
	}, time.Second, 10*time.Millisecond)

	select {
	case prompt := <-promptSeen:
		require.Contains(t, prompt, "Recent events from the scoped incident timeline")
		require.Contains(t, prompt, "node=media-node")
		require.Contains(t, prompt, "Log: rss sync panic")
		require.Contains(t, prompt, "Log: unit entered failed state")
		require.Contains(t, prompt, timelineID)
	default:
		t.Fatal("expected Ollama correlation prompt")
	}
}

func TestCorrelateAsync_DropsHallucinatedEntryIDs(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaModelKey, Value: "llama3.2"}).Error)

	now := time.Now().UTC()
	incidentID := ulid.Make().String()
	realID := ulid.Make().String()
	fakeID := ulid.Make().String()

	require.NoError(t, database.Create(&types.Entry{
		ID:        realID,
		Timestamp: now.Add(-time.Minute),
		NodeName:  "node-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "die",
		Content:   "real entry",
		Metadata:  `{}`,
	}).Error)
	require.NoError(t, database.Create(&models.Incident{
		ID:         incidentID,
		OpenedAt:   now,
		Status:     "open",
		Confidence: "suspected",
		Title:      "nginx crash",
		Services:   `["nginx"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{}`,
	}).Error)

	originalCall := callOllamaCorrelateFunc
	originalDelay := correlateDelay
	callOllamaCorrelateFunc = func(baseURL, model, prompt string) (string, error) {
		return `{"summary":"something","causes":[{"entry_id":"` + fakeID + `","confidence":0.9,"reason":"ghost"}]}`, nil
	}
	correlateDelay = 0
	defer func() {
		callOllamaCorrelateFunc = originalCall
		correlateDelay = originalDelay
	}()

	enricher := NewOllamaEnricher(database, nil)
	enricher.CorrelateAsync(incidentID, nil, "node-01")

	require.Eventually(t, func() bool {
		var inc models.Incident
		if err := database.First(&inc, "id = ?", incidentID).Error; err != nil {
			return false
		}
		meta := parseIncidentTestMetadata(t, inc.Metadata)
		_, pending := meta["ai_pending"]
		return !pending
	}, time.Second, 10*time.Millisecond)

	var links []models.IncidentEntry
	require.NoError(t, database.Where("incident_id = ? AND role = ?", incidentID, "ai_cause").Find(&links).Error)
	require.Empty(t, links)
}

func parseIncidentTestMetadata(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	meta := make(map[string]interface{})
	require.NoError(t, json.Unmarshal([]byte(raw), &meta))
	return meta
}
