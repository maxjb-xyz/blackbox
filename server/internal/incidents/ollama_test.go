package incidents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
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

	require.NoError(t, database.Create(&models.AppSetting{Key: aiURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: aiModelKey, Value: "llama3.2"}).Error)

	promptSeen := make(chan string, 1)
	originalCall := callGenerateFunc
	callGenerateFunc = func(_ context.Context, provider LLMProvider, model, prompt string, timeout time.Duration) (string, error) {
		promptSeen <- prompt
		return "Crash caused by invalid config", nil
	}
	defer func() { callGenerateFunc = originalCall }()

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

	manager := NewManager(database, nil, nil)
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
		require.Contains(t, prompt, "concise but detailed root cause analysis")
		require.Contains(t, prompt, "[trigger] docker/die")
		require.Contains(t, prompt, "Log: fatal: invalid nginx.conf")
		require.Contains(t, prompt, "Root cause summary in 2-4 sentences")
		require.True(t, strings.Contains(prompt, "Confidence: suspected"), prompt)
	default:
		t.Fatal("expected Ollama prompt for suspected incident")
	}
}

func TestAIEnricher_SetsAndClearsPendingState(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	release := make(chan struct{})
	originalCall := callGenerateFunc
	callGenerateFunc = func(_ context.Context, provider LLMProvider, model, prompt string, timeout time.Duration) (string, error) {
		<-release
		return "Root cause: bad config", nil
	}
	defer func() { callGenerateFunc = originalCall }()

	require.NoError(t, database.Create(&models.AppSetting{Key: aiURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: aiModelKey, Value: "llama3.2"}).Error)

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

	enricher := NewAIEnricher(database, nil)
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
		return pending && meta["ai_model"] == "llama3.2" && meta["ai_mode"] == "analysis"
	}, time.Second, 10*time.Millisecond)

	close(release)

	require.Eventually(t, func() bool {
		var inc models.Incident
		if err := database.First(&inc, "id = ?", incidentID).Error; err != nil {
			return false
		}
		meta := parseIncidentTestMetadata(t, inc.Metadata)
		_, pending := meta["ai_pending"]
		return !pending && meta["ai_analysis"] == "Root cause: bad config" && meta["ai_mode"] == "analysis"
	}, time.Second, 10*time.Millisecond)
}

func TestCorrelateAsync_WritesAICauseLinks(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	require.NoError(t, database.Create(&models.AppSetting{Key: aiURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: aiModelKey, Value: "llama3.2"}).Error)

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

	originalCall := callCorrelateGenerateFunc
	originalDelay := correlateDelay
	callCorrelateGenerateFunc = func(_ context.Context, provider LLMProvider, model, prompt string, timeout time.Duration) (string, error) {
		return `analysis wrapper {"summary":"nginx crashed due to resource exhaustion","causes":[{"entry_id":"` + candidateID + `","confidence":0.85,"reason":"Container exited before the outage"}]} done`, nil
	}
	correlateDelay = 0
	defer func() {
		callCorrelateGenerateFunc = originalCall
		correlateDelay = originalDelay
	}()

	enricher := NewAIEnricher(database, nil)
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
	require.Equal(t, "enhanced", meta["ai_mode"])
}

func TestCorrelateAsync_UsesScopedIncidentNodesAndSetsVerified(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	require.NoError(t, database.Create(&models.AppSetting{Key: aiURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: aiModelKey, Value: "llama3.2"}).Error)

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
	originalCall := callCorrelateGenerateFunc
	originalDelay := correlateDelay
	callCorrelateGenerateFunc = func(_ context.Context, provider LLMProvider, model, prompt string, timeout time.Duration) (string, error) {
		promptSeen <- prompt
		return `{"summary":"radarr failed on media-node","verified":true,"causes":[]}`, nil
	}
	correlateDelay = 0
	defer func() {
		callCorrelateGenerateFunc = originalCall
		correlateDelay = originalDelay
	}()

	enricher := NewAIEnricher(database, nil)
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
		require.Contains(t, prompt, "\"summary\": \"<concise but detailed 2-4 sentence root cause summary>\"")
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

	require.NoError(t, database.Create(&models.AppSetting{Key: aiURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: aiModelKey, Value: "llama3.2"}).Error)

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

	originalCall := callCorrelateGenerateFunc
	originalDelay := correlateDelay
	callCorrelateGenerateFunc = func(_ context.Context, provider LLMProvider, model, prompt string, timeout time.Duration) (string, error) {
		return `{"summary":"something","causes":[{"entry_id":"` + fakeID + `","confidence":0.9,"reason":"ghost"}]}`, nil
	}
	correlateDelay = 0
	defer func() {
		callCorrelateGenerateFunc = originalCall
		correlateDelay = originalDelay
	}()

	enricher := NewAIEnricher(database, nil)
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

func TestCorrelationScopeNodes_DropsEmptyFallback(t *testing.T) {
	require.Empty(t, correlationScopeNodes("", nil, "   "))
	require.Equal(t, []string{"node-01"}, correlationScopeNodes("", nil, " node-01 "))
	require.Equal(t, []string{"node-01"}, correlationScopeNodes(`[""]`, nil, " node-01 "))
}

func TestAIEnricher_LoadAIConfig_FallsBackToLegacyOllamaSettings(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaModelKey, Value: "llama3.2"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: ollamaModeKey, Value: "enhanced"}).Error)

	enricher := NewAIEnricher(database, nil)
	cfg := enricher.loadAIConfig()

	require.Equal(t, "llama3.2", cfg.model)
	require.Equal(t, "enhanced", cfg.mode)

	provider, ok := cfg.provider.(*ollamaProvider)
	require.True(t, ok)
	require.Equal(t, "http://ollama.local", provider.baseURL)
}

func TestCorrelateAsync_ExcludesPriorAICauseLinksFromDeterministicPrompt(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	require.NoError(t, database.Create(&models.AppSetting{Key: aiURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: aiModelKey, Value: "llama3.2"}).Error)

	now := time.Now().UTC()
	triggerID := ulid.Make().String()
	causeID := ulid.Make().String()
	oldAIID := ulid.Make().String()

	require.NoError(t, database.Create(&types.Entry{
		ID:        triggerID,
		Timestamp: now,
		NodeName:  "node-01",
		Source:    "webhook",
		Service:   "nginx",
		Event:     "down",
		Content:   "nginx monitor down",
		Metadata:  `{}`,
	}).Error)
	require.NoError(t, database.Create(&types.Entry{
		ID:        causeID,
		Timestamp: now.Add(-30 * time.Second),
		NodeName:  "node-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "stop",
		Content:   "container stopped",
		Metadata:  `{}`,
	}).Error)
	require.NoError(t, database.Create(&types.Entry{
		ID:        oldAIID,
		Timestamp: now.Add(-20 * time.Second),
		NodeName:  "node-01",
		Source:    "systemd",
		Service:   "nginx.service",
		Event:     "failed",
		Content:   "old ai cause",
		Metadata:  `{}`,
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
		Metadata:   `{"ai_verified":true}`,
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
	require.NoError(t, database.Create(&models.IncidentEntry{
		IncidentID: incidentID,
		EntryID:    oldAIID,
		Role:       "ai_cause",
		Score:      70,
		Reason:     "old ai output",
	}).Error)

	promptSeen := make(chan string, 1)
	originalCall := callCorrelateGenerateFunc
	originalDelay := correlateDelay
	callCorrelateGenerateFunc = func(_ context.Context, provider LLMProvider, model, prompt string, timeout time.Duration) (string, error) {
		promptSeen <- prompt
		return `{"summary":"nginx down","verified":true,"causes":[]}`, nil
	}
	correlateDelay = 0
	defer func() {
		callCorrelateGenerateFunc = originalCall
		correlateDelay = originalDelay
	}()

	enricher := NewAIEnricher(database, nil)
	enricher.CorrelateAsync(incidentID, nil, "node-01")

	var prompt string
	require.Eventually(t, func() bool {
		select {
		case prompt = <-promptSeen:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	require.NotContains(t, prompt, "old ai cause")
	require.NotContains(t, prompt, "[ai_cause")
	require.Contains(t, prompt, "[cause score=90]")
}

func TestAIEnricher_QueuesDuplicateDispatchesWithoutConcurrentCalls(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)

	require.NoError(t, database.Create(&models.AppSetting{Key: aiURLKey, Value: "http://ollama.local"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: aiModelKey, Value: "llama3.2"}).Error)

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

	originalCall := callGenerateFunc
	defer func() { callGenerateFunc = originalCall }()

	releaseFirst := make(chan struct{})
	var calls atomic.Int32
	var active atomic.Int32
	var maxActive atomic.Int32

	callGenerateFunc = func(_ context.Context, provider LLMProvider, model, prompt string, timeout time.Duration) (string, error) {
		callNum := calls.Add(1)
		currentActive := active.Add(1)
		for {
			seen := maxActive.Load()
			if currentActive <= seen || maxActive.CompareAndSwap(seen, currentActive) {
				break
			}
		}
		defer active.Add(-1)

		if callNum == 1 {
			<-releaseFirst
		}
		return fmt.Sprintf("analysis %d", callNum), nil
	}

	enricher := NewAIEnricher(database, nil)
	enricher.EnrichAsync(incidentID, []enrichEntry{{Role: "trigger", Content: "first", Source: "docker", Event: "die"}})

	require.Eventually(t, func() bool {
		return calls.Load() == 1
	}, time.Second, 10*time.Millisecond)

	enricher.EnrichAsync(incidentID, []enrichEntry{{Role: "trigger", Content: "second", Source: "docker", Event: "restart"}})

	require.Never(t, func() bool {
		return calls.Load() > 1
	}, 150*time.Millisecond, 10*time.Millisecond)

	close(releaseFirst)

	require.Eventually(t, func() bool {
		var inc models.Incident
		if err := database.First(&inc, "id = ?", incidentID).Error; err != nil {
			return false
		}
		meta := parseIncidentTestMetadata(t, inc.Metadata)
		return calls.Load() == 2 && meta["ai_analysis"] == "analysis 2"
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, int32(1), maxActive.Load())
}

func parseIncidentTestMetadata(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	meta := make(map[string]interface{})
	require.NoError(t, json.Unmarshal([]byte(raw), &meta))
	return meta
}
