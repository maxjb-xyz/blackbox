package incidents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"gorm.io/gorm"
)

const ollamaTimeout = 60 * time.Second
const correlateOllamaTimeout = 90 * time.Second
const ollamaURLKey = "ollama_url"
const ollamaModelKey = "ollama_model"
const ollamaModeKey = "ollama_mode"

var correlateDelay = 3 * time.Second

var callOllamaFunc = func(baseURL, model, prompt string) (string, error) {
	return callOllamaWithTimeout(baseURL, model, prompt, ollamaTimeout)
}

var callOllamaCorrelateFunc = func(baseURL, model, prompt string) (string, error) {
	return callOllamaWithTimeout(baseURL, model, prompt, correlateOllamaTimeout)
}

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

type correlationResponse struct {
	Summary string `json:"summary"`
	Causes  []struct {
		EntryID    string  `json:"entry_id"`
		Confidence float64 `json:"confidence"`
		Reason     string  `json:"reason"`
	} `json:"causes"`
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

// DispatchOllamaAsync routes the incident to the configured Ollama mode.
func (m *Manager) DispatchOllamaAsync(incidentID string, linkedEntries []enrichEntry, nodeName string) {
	if m == nil || m.enricher == nil {
		return
	}
	if m.enricher.loadOllamaMode() == "enhanced" {
		m.enricher.CorrelateAsync(incidentID, linkedEntries, nodeName)
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
	ollamaURL, ollamaModel := e.loadOllamaConfig()
	if ollamaURL == "" || ollamaModel == "" {
		return
	}
	if !e.setPending(incidentID, ollamaModel) {
		return
	}
	go e.enrich(incidentID, linkedEntries, ollamaURL, ollamaModel)
}

// CorrelateAsync spawns a goroutine that waits for deterministic links to
// settle, then queries Ollama for additional AI-derived causes.
func (e *OllamaEnricher) CorrelateAsync(incidentID string, _ []enrichEntry, nodeName string) {
	if e == nil {
		return
	}
	ollamaURL, ollamaModel := e.loadOllamaConfig()
	if ollamaURL == "" || ollamaModel == "" {
		return
	}
	if !e.setPending(incidentID, ollamaModel) {
		return
	}
	go e.correlate(incidentID, nodeName, ollamaURL, ollamaModel)
}

func (e *OllamaEnricher) enrich(incidentID string, entries []enrichEntry, ollamaURL, ollamaModel string) {
	var inc models.Incident
	if err := e.db.First(&inc, "id = ?", incidentID).Error; err != nil {
		e.clearPending(incidentID)
		return
	}

	prompt := buildPrompt(inc, entries)
	result, err := callOllamaFunc(ollamaURL, ollamaModel, prompt)
	if err != nil {
		log.Printf("incidents: ollama enrichment failed for %s: %v", incidentID, err)
		e.clearPending(incidentID)
		return
	}

	e.updateIncidentMetadata(incidentID, func(meta map[string]interface{}) {
		delete(meta, "ai_pending")
		meta["ai_analysis"] = result
		meta["ai_model"] = ollamaModel
		meta["ai_enriched_at"] = time.Now().UTC().Format(time.RFC3339)
	})
}

func (e *OllamaEnricher) correlate(incidentID, nodeName, ollamaURL, ollamaModel string) {
	time.Sleep(correlateDelay)

	var inc models.Incident
	if err := e.db.First(&inc, "id = ?", incidentID).Error; err != nil {
		e.clearPending(incidentID)
		return
	}

	var detLinks []models.IncidentEntry
	if err := e.db.Where("incident_id = ?", incidentID).Find(&detLinks).Error; err != nil {
		log.Printf("incidents: correlate load links for %s: %v", incidentID, err)
		e.clearPending(incidentID)
		return
	}

	detEntryIDs := make([]string, 0, len(detLinks))
	for _, link := range detLinks {
		if strings.TrimSpace(link.EntryID) == "" {
			continue
		}
		detEntryIDs = append(detEntryIDs, link.EntryID)
	}

	var detEntries []types.Entry
	if len(detEntryIDs) > 0 {
		if err := e.db.Where("id IN ?", detEntryIDs).Order("timestamp ASC").Find(&detEntries).Error; err != nil {
			log.Printf("incidents: correlate load deterministic entries for %s: %v", incidentID, err)
			e.clearPending(incidentID)
			return
		}
	}

	windowStart := inc.OpenedAt.Add(-5 * time.Minute)
	windowEnd := inc.OpenedAt.Add(time.Minute)
	query := e.db.Where(
		"timestamp BETWEEN ? AND ? AND NOT (source = ? AND event IN ?)",
		windowStart,
		windowEnd,
		"webhook",
		[]string{"down", "up"},
	)
	if strings.TrimSpace(nodeName) != "" {
		query = query.Where("node_name = ?", nodeName)
	}

	var nodeEntries []types.Entry
	if err := query.Order("timestamp DESC").Limit(100).Find(&nodeEntries).Error; err != nil {
		log.Printf("incidents: correlate load node entries for %s: %v", incidentID, err)
		e.clearPending(incidentID)
		return
	}

	prompt := buildCorrelationPrompt(inc, detLinks, detEntries, nodeEntries)
	result, err := callOllamaCorrelateFunc(ollamaURL, ollamaModel, prompt)
	if err != nil {
		log.Printf("incidents: ollama correlation failed for %s: %v", incidentID, err)
		e.clearPending(incidentID)
		return
	}

	var response correlationResponse
	if err := json.Unmarshal([]byte(extractJSON(result)), &response); err != nil {
		log.Printf("incidents: correlate parse response for %s: %v", incidentID, err)
		e.clearPending(incidentID)
		return
	}

	if err := e.db.Where("incident_id = ? AND role = ?", incidentID, "ai_cause").Delete(&models.IncidentEntry{}).Error; err != nil {
		log.Printf("incidents: correlate clear ai_cause links for %s: %v", incidentID, err)
	}

	validIDs := make(map[string]struct{}, len(detEntries)+len(nodeEntries))
	for _, entry := range detEntries {
		validIDs[entry.ID] = struct{}{}
	}
	for _, entry := range nodeEntries {
		validIDs[entry.ID] = struct{}{}
	}

	for _, cause := range response.Causes {
		cause.EntryID = strings.TrimSpace(cause.EntryID)
		if cause.EntryID == "" {
			continue
		}
		if _, ok := validIDs[cause.EntryID]; !ok {
			log.Printf("incidents: correlate dropped hallucinated entry_id %s for incident %s", cause.EntryID, incidentID)
			continue
		}

		var existing models.IncidentEntry
		err := e.db.Where("incident_id = ? AND entry_id = ?", incidentID, cause.EntryID).First(&existing).Error
		if err == nil && existing.Role != "ai_cause" {
			log.Printf("incidents: correlate skipped existing deterministic link %s for incident %s", cause.EntryID, incidentID)
			continue
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("incidents: correlate lookup link %s for incident %s: %v", cause.EntryID, incidentID, err)
			continue
		}

		score := int(cause.Confidence * 100)
		if score < 0 {
			score = 0
		}
		if score > 100 {
			score = 100
		}

		link := models.IncidentEntry{
			IncidentID: incidentID,
			EntryID:    cause.EntryID,
			Role:       "ai_cause",
			Score:      score,
			Reason:     sanitizeExternalText(cause.Reason),
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := e.db.Create(&link).Error; err != nil {
				log.Printf("incidents: correlate write ai_cause link %s->%s: %v", incidentID, cause.EntryID, err)
			}
			continue
		}
		if err := e.db.Model(&models.IncidentEntry{}).
			Where("incident_id = ? AND entry_id = ?", incidentID, cause.EntryID).
			Updates(map[string]interface{}{"role": link.Role, "score": link.Score, "reason": link.Reason}).Error; err != nil {
			log.Printf("incidents: correlate update ai_cause link %s->%s: %v", incidentID, cause.EntryID, err)
		}
	}

	e.updateIncidentMetadata(incidentID, func(meta map[string]interface{}) {
		delete(meta, "ai_pending")
		if summary := sanitizeExternalText(response.Summary); summary != "" {
			meta["ai_analysis"] = summary
		}
		meta["ai_model"] = ollamaModel
		meta["ai_enriched_at"] = time.Now().UTC().Format(time.RFC3339)
	})
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

func (e *OllamaEnricher) loadOllamaMode() string {
	var setting models.AppSetting
	if err := e.db.First(&setting, "key = ?", ollamaModeKey).Error; err != nil {
		return "analysis"
	}
	if strings.TrimSpace(setting.Value) == "enhanced" {
		return "enhanced"
	}
	return "analysis"
}

func (e *OllamaEnricher) setPending(incidentID, ollamaModel string) bool {
	return e.updateIncidentMetadata(incidentID, func(meta map[string]interface{}) {
		meta["ai_pending"] = true
		meta["ai_model"] = ollamaModel
	})
}

func (e *OllamaEnricher) clearPending(incidentID string) {
	e.updateIncidentMetadata(incidentID, func(meta map[string]interface{}) {
		delete(meta, "ai_pending")
	})
}

func (e *OllamaEnricher) updateIncidentMetadata(incidentID string, apply func(map[string]interface{})) bool {
	var inc models.Incident
	if err := e.db.First(&inc, "id = ?", incidentID).Error; err != nil {
		return false
	}

	meta := make(map[string]interface{})
	if err := json.Unmarshal([]byte(inc.Metadata), &meta); err != nil {
		meta = make(map[string]interface{})
	}
	apply(meta)

	metaBytes, _ := json.Marshal(meta)
	if err := e.db.Model(&models.Incident{}).Where("id = ?", incidentID).
		Update("metadata", string(metaBytes)).Error; err != nil {
		log.Printf("incidents: save ai metadata for %s: %v", incidentID, err)
		return false
	}
	if e.onIncidentSet != nil {
		e.onIncidentSet(incidentID)
	}
	return true
}

func buildPrompt(inc models.Incident, entries []enrichEntry) string {
	var b strings.Builder
	b.WriteString("You are analyzing a server incident. Provide a concise root cause analysis.\n\n")
	fmt.Fprintf(&b, "Incident: %s\n", sanitizeExternalText(inc.Title))
	fmt.Fprintf(&b, "Status: %s | Confidence: %s\n\n", inc.Status, inc.Confidence)
	b.WriteString("Events (chronological):\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "- [%s] %s/%s: %s\n", e.Role, e.Source, e.Event, sanitizeExternalText(e.Content))
		if e.Log != "" {
			fmt.Fprintf(&b, "  Log: %s\n", sanitizeExternalText(truncate(e.Log, 300)))
		}
	}
	b.WriteString("\nProvide: 1) Root cause in one sentence. 2) Why you think so. 3) A better incident title if applicable.\n")
	return b.String()
}

func buildCorrelationPrompt(inc models.Incident, detLinks []models.IncidentEntry, detEntries []types.Entry, nodeEntries []types.Entry) string {
	var b strings.Builder
	b.WriteString("You are analyzing a server incident. Return ONLY valid JSON with no prose or markdown.\n\n")
	fmt.Fprintf(&b, "Incident: %s\n", sanitizeExternalText(inc.Title))
	fmt.Fprintf(&b, "Status: %s | Confidence: %s | Opened: %s\n\n", inc.Status, inc.Confidence, inc.OpenedAt.UTC().Format(time.RFC3339))

	roleByID := make(map[string]string, len(detLinks))
	scoreByID := make(map[string]int, len(detLinks))
	for _, link := range detLinks {
		roleByID[link.EntryID] = link.Role
		scoreByID[link.EntryID] = link.Score
	}

	if len(detEntries) > 0 {
		b.WriteString("Deterministic links already identified by the correlation engine:\n")
		for _, entry := range detEntries {
			fmt.Fprintf(&b, "- [%s score=%d id=%s svc=%s] %s/%s (%s): %s\n",
				roleByID[entry.ID],
				scoreByID[entry.ID],
				entry.ID,
				entry.Service,
				entry.Source,
				entry.Event,
				entry.Timestamp.UTC().Format(time.RFC3339),
				sanitizeExternalText(truncate(entry.Content, 200)),
			)
		}
		b.WriteString("\n")
	}

	b.WriteString("Recent events on this node. Use entry_id values exactly as written if you identify additional causes:\n")
	for _, entry := range nodeEntries {
		fmt.Fprintf(&b, "- [id=%s svc=%s] %s/%s (%s): %s\n",
			entry.ID,
			entry.Service,
			entry.Source,
			entry.Event,
			entry.Timestamp.UTC().Format(time.RFC3339),
			sanitizeExternalText(truncate(entry.Content, 200)),
		)
	}

	b.WriteString(`
Return JSON exactly matching this schema:
{
  "summary": "<one sentence root cause summary>",
  "causes": [
    {"entry_id": "<verbatim id from the events above>", "confidence": 0.0, "reason": "<why this event caused the incident>"}
  ]
}
If no additional causes are found beyond the deterministic links, return an empty causes array.`)

	return b.String()
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		return s
	}
	return s[start : end+1]
}

func callOllamaWithTimeout(baseURL, model, prompt string, timeout time.Duration) (string, error) {
	reqBody, _ := json.Marshal(ollamaRequest{Model: model, Prompt: prompt, Stream: false})
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(baseURL+"/api/generate", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("ollama responded with %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var result ollamaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	response := strings.TrimSpace(result.Response)
	if response == "" {
		return "", errors.New("ollama response was empty")
	}
	return response, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func sanitizeExternalText(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = redactSensitiveAssignment(strings.TrimSpace(line))
	}
	return strings.Join(lines, "\n")
}

func redactSensitiveAssignment(line string) string {
	for _, separator := range []string{"=", ":"} {
		idx := strings.Index(line, separator)
		if idx == -1 {
			continue
		}
		key := normalizeSensitiveKey(line[:idx])
		if key == "" || !looksSensitiveKey(key) {
			continue
		}
		return strings.TrimSpace(line[:idx+1]) + " [REDACTED]"
	}
	return line
}

func normalizeSensitiveKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "")
	return replacer.Replace(value)
}

func looksSensitiveKey(key string) bool {
	for _, fragment := range []string{"token", "secret", "password", "apikey", "clientsecret", "authorization", "bearer"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}
