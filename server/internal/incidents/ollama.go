package incidents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

const ollamaTimeout = 60 * time.Second
const ollamaURLKey = "ollama_url"
const ollamaModelKey = "ollama_model"

type OllamaEnricher struct {
	db            *gorm.DB
	onIncidentSet func(string)
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

type enrichEntry struct {
	Role    string
	Content string
	Source  string
	Event   string
	Log     string
}

func NewOllamaEnricher(db *gorm.DB, onIncidentSet func(string)) *OllamaEnricher {
	return &OllamaEnricher{
		db:            db,
		onIncidentSet: onIncidentSet,
	}
}

// EnrichAsync spawns a goroutine to enrich the incident with Ollama.
func (m *Manager) EnrichAsync(incidentID string, linkedEntries []enrichEntry) {
	if m == nil || m.enricher == nil {
		return
	}
	m.enricher.EnrichAsync(incidentID, linkedEntries)
}

// EnrichAsync spawns a goroutine to enrich the incident with Ollama.
// Safe to call with m.mu held — it immediately returns; the goroutine
// acquires its own resources.
func (e *OllamaEnricher) EnrichAsync(incidentID string, linkedEntries []enrichEntry) {
	if e == nil {
		return
	}
	go e.enrich(incidentID, linkedEntries)
}

func (e *OllamaEnricher) enrich(incidentID string, entries []enrichEntry) {
	ollamaURL, ollamaModel := e.loadOllamaConfig()
	if ollamaURL == "" || ollamaModel == "" {
		return
	}

	var inc models.Incident
	if err := e.db.First(&inc, "id = ?", incidentID).Error; err != nil {
		return
	}

	prompt := buildPrompt(inc, entries)
	result, err := callOllama(ollamaURL, ollamaModel, prompt)
	if err != nil {
		log.Printf("incidents: ollama enrichment failed for %s: %v", incidentID, err)
		return
	}

	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(inc.Metadata), &meta); err != nil {
		meta = make(map[string]interface{})
	}
	meta["ai_analysis"] = result
	meta["ai_model"] = ollamaModel
	meta["ai_enriched_at"] = time.Now().UTC().Format(time.RFC3339)

	metaBytes, _ := json.Marshal(meta)
	if err := e.db.Model(&models.Incident{}).Where("id = ?", incidentID).
		Update("metadata", string(metaBytes)).Error; err != nil {
		log.Printf("incidents: save ai metadata for %s: %v", incidentID, err)
		return
	}

	if e.onIncidentSet != nil {
		e.onIncidentSet(incidentID)
	}
}

func (e *OllamaEnricher) loadOllamaConfig() (url, model string) {
	var settings []models.AppSetting
	e.db.Where("key IN ?", []string{ollamaURLKey, ollamaModelKey}).Find(&settings)
	for _, s := range settings {
		switch s.Key {
		case ollamaURLKey:
			url = strings.TrimSpace(s.Value)
		case ollamaModelKey:
			model = strings.TrimSpace(s.Value)
		}
	}
	return
}

func buildPrompt(inc models.Incident, entries []enrichEntry) string {
	var b strings.Builder
	b.WriteString("You are analyzing a server incident. Provide a concise root cause analysis.\n\n")
	fmt.Fprintf(&b, "Incident: %s\n", inc.Title)
	fmt.Fprintf(&b, "Status: %s | Confidence: %s\n\n", inc.Status, inc.Confidence)
	b.WriteString("Events (chronological):\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "- [%s] %s/%s: %s\n", e.Role, e.Source, e.Event, e.Content)
		if e.Log != "" {
			fmt.Fprintf(&b, "  Log: %s\n", truncate(e.Log, 300))
		}
	}
	b.WriteString("\nProvide: 1) Root cause in one sentence. 2) Why you think so. 3) A better incident title if applicable.\n")
	return b.String()
}

func callOllama(baseURL, model, prompt string) (string, error) {
	reqBody, _ := json.Marshal(ollamaRequest{Model: model, Prompt: prompt, Stream: false})
	client := &http.Client{Timeout: ollamaTimeout}
	resp, err := client.Post(baseURL+"/api/generate", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}
	var result ollamaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Response), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
