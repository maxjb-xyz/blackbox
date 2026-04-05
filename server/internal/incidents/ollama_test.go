package incidents

import (
	"encoding/json"
	"testing"
	"time"

	"blackbox/server/internal/db"
	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

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

func parseIncidentTestMetadata(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	meta := make(map[string]interface{})
	require.NoError(t, json.Unmarshal([]byte(raw), &meta))
	return meta
}
