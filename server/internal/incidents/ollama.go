package incidents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"gorm.io/gorm"
)

// New AI provider settings keys
const aiProviderKey = "ai_provider"
const aiURLKey = "ai_url"
const aiModelKey = "ai_model"
const aiAPIKeyKey = "ai_api_key"
const aiModeKey = "ai_mode"

// Legacy Ollama keys — read-only fallback for existing installs
const ollamaURLKey = "ollama_url"
const ollamaModelKey = "ollama_model"
const ollamaModeKey = "ollama_mode"

const aiTimeout = 120 * time.Second
const aiCorrelateTimeout = 180 * time.Second

var correlateDelay = 3 * time.Second

var callGenerateFunc = func(ctx context.Context, provider LLMProvider, model, prompt string, timeout time.Duration) (string, error) {
	return provider.Generate(ctx, model, prompt, timeout)
}

var callCorrelateGenerateFunc = func(ctx context.Context, provider LLMProvider, model, prompt string, timeout time.Duration) (string, error) {
	return provider.Generate(ctx, model, prompt, timeout)
}

type AIEnricher struct {
	db            *gorm.DB
	onIncidentSet func(string)
	dispatchMu    sync.Mutex
	dispatches    map[string]aiDispatchState
}

type aiDispatch struct {
	incidentID    string
	mode          string
	linkedEntries []enrichEntry
	nodeName      string
	provider      LLMProvider
	model         string
}

type aiDispatchState struct {
	queued *aiDispatch
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
	Summary     string       `json:"summary"`
	Verified    *bool        `json:"verified"`
	Findings    []Finding    `json:"findings"`
	Annotations []Annotation `json:"annotations"`
	Causes      []Cause      `json:"causes"`
}

type Finding struct {
	Kind       string   `json:"kind"`
	Confidence float64  `json:"confidence"`
	Title      string   `json:"title"`
	Detail     string   `json:"detail"`
	Evidence   []string `json:"evidence"`
}

type Annotation struct {
	EntryID    string   `json:"entry_id"`
	Kind       string   `json:"kind"`
	Confidence float64  `json:"confidence"`
	Title      string   `json:"title"`
	Detail     string   `json:"detail"`
	Evidence   []string `json:"evidence"`
}

type Cause struct {
	EntryID    string  `json:"entry_id"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type aiFindingMetadata struct {
	Kind       string   `json:"kind"`
	Confidence int      `json:"confidence"`
	Title      string   `json:"title"`
	Detail     string   `json:"detail"`
	Evidence   []string `json:"evidence,omitempty"`
}

type aiAnnotationMetadata struct {
	EntryID    string   `json:"entry_id"`
	Kind       string   `json:"kind"`
	Confidence int      `json:"confidence"`
	Title      string   `json:"title"`
	Detail     string   `json:"detail"`
	Evidence   []string `json:"evidence,omitempty"`
}

type enrichEntry struct {
	Role    string
	Content string
	Source  string
	Event   string
	Log     string
}

type aiConfig struct {
	provider LLMProvider
	model    string
	mode     string
}

func NewAIEnricher(db *gorm.DB, onIncidentSet func(string)) *AIEnricher {
	return &AIEnricher{
		db:            db,
		onIncidentSet: onIncidentSet,
		dispatches:    make(map[string]aiDispatchState),
	}
}

// EnrichAsync delegates to the AIEnricher.
func (m *Manager) EnrichAsync(incidentID string, linkedEntries []enrichEntry) {
	if m == nil || m.enricher == nil {
		return
	}
	m.enricher.EnrichAsync(incidentID, linkedEntries)
}

// DispatchAIAsync routes the incident to the configured AI mode.
func (m *Manager) DispatchAIAsync(incidentID string, linkedEntries []enrichEntry, nodeName string) {
	if m == nil || m.enricher == nil {
		return
	}
	if m.enricher.loadAIMode() == "enhanced" {
		m.enricher.CorrelateAsync(incidentID, linkedEntries, nodeName)
		return
	}
	m.enricher.EnrichAsync(incidentID, linkedEntries)
}

// EnrichAsync spawns a goroutine to enrich the incident with AI analysis.
// Safe to call with m.mu held — it immediately returns; the goroutine
// acquires its own resources.
func (e *AIEnricher) EnrichAsync(incidentID string, linkedEntries []enrichEntry) {
	if e == nil {
		return
	}
	cfg := e.loadAIConfig()
	if cfg.provider == nil || cfg.model == "" {
		return
	}
	e.enqueueDispatch(aiDispatch{
		incidentID:    incidentID,
		mode:          "analysis",
		linkedEntries: linkedEntries,
		provider:      cfg.provider,
		model:         cfg.model,
	})
}

// CorrelateAsync spawns a goroutine that waits for deterministic links to
// settle, then queries the configured AI provider for additional AI-derived causes.
func (e *AIEnricher) CorrelateAsync(incidentID string, _ []enrichEntry, nodeName string) {
	if e == nil {
		return
	}
	cfg := e.loadAIConfig()
	if cfg.provider == nil || cfg.model == "" {
		return
	}
	e.enqueueDispatch(aiDispatch{
		incidentID: incidentID,
		mode:       "enhanced",
		nodeName:   nodeName,
		provider:   cfg.provider,
		model:      cfg.model,
	})
}

func (e *AIEnricher) enqueueDispatch(dispatch aiDispatch) {
	if e == nil {
		return
	}
	if !e.registerDispatch(dispatch) {
		return
	}
	go e.runDispatchLoop(dispatch)
}

func (e *AIEnricher) registerDispatch(dispatch aiDispatch) bool {
	e.dispatchMu.Lock()
	defer e.dispatchMu.Unlock()

	state, ok := e.dispatches[dispatch.incidentID]
	if ok {
		state.queued = &dispatch
		e.dispatches[dispatch.incidentID] = state
		return false
	}

	e.dispatches[dispatch.incidentID] = aiDispatchState{}
	return true
}

func (e *AIEnricher) nextQueuedDispatch(incidentID string) (aiDispatch, bool) {
	e.dispatchMu.Lock()
	defer e.dispatchMu.Unlock()

	state, ok := e.dispatches[incidentID]
	if !ok {
		return aiDispatch{}, false
	}
	if state.queued == nil {
		delete(e.dispatches, incidentID)
		return aiDispatch{}, false
	}

	next := *state.queued
	state.queued = nil
	e.dispatches[incidentID] = state
	return next, true
}

func (e *AIEnricher) runDispatchLoop(dispatch aiDispatch) {
	current := dispatch
	for {
		if !e.setPending(current.incidentID, current.model, current.mode) {
			if next, ok := e.nextQueuedDispatch(current.incidentID); ok {
				current = next
				continue
			}
			return
		}

		if current.mode == "enhanced" {
			e.correlate(current)
		} else {
			e.enrich(current)
		}

		next, ok := e.nextQueuedDispatch(current.incidentID)
		if !ok {
			return
		}
		current = next
	}
}

func (e *AIEnricher) enrich(dispatch aiDispatch) {
	var inc models.Incident
	if err := e.db.First(&inc, "id = ?", dispatch.incidentID).Error; err != nil {
		e.clearPending(dispatch.incidentID)
		return
	}

	prompt := buildPrompt(inc, dispatch.linkedEntries)
	result, err := callGenerateFunc(context.Background(), dispatch.provider, dispatch.model, prompt, aiTimeout)
	if err != nil {
		log.Printf("incidents: ai enrichment failed for %s: %v", dispatch.incidentID, err)
		e.clearPending(dispatch.incidentID)
		return
	}

	if !e.updateIncidentMetadata(dispatch.incidentID, func(meta map[string]interface{}) {
		delete(meta, "ai_pending")
		meta["ai_analysis"] = result
		meta["ai_model"] = dispatch.model
		meta["ai_mode"] = dispatch.mode
		meta["ai_enriched_at"] = time.Now().UTC().Format(time.RFC3339)
	}) {
		return
	}
}

func (e *AIEnricher) correlate(dispatch aiDispatch) {
	time.Sleep(correlateDelay)

	var inc models.Incident
	if err := e.db.First(&inc, "id = ?", dispatch.incidentID).Error; err != nil {
		e.clearPending(dispatch.incidentID)
		return
	}

	var detLinks []models.IncidentEntry
	if err := e.db.Where("incident_id = ?", dispatch.incidentID).Find(&detLinks).Error; err != nil {
		log.Printf("incidents: correlate load links for %s: %v", dispatch.incidentID, err)
		e.clearPending(dispatch.incidentID)
		return
	}

	excludedNodeEntryIDs := make(map[string]struct{}, len(detLinks))
	detEntryIDs := make([]string, 0, len(detLinks))
	filteredDetLinks := detLinks[:0]
	for _, link := range detLinks {
		entryID := strings.TrimSpace(link.EntryID)
		if entryID == "" {
			continue
		}
		if link.Role == "ai_cause" {
			excludedNodeEntryIDs[entryID] = struct{}{}
			continue
		}
		filteredDetLinks = append(filteredDetLinks, link)
		detEntryIDs = append(detEntryIDs, entryID)
	}
	detLinks = filteredDetLinks

	var detEntries []types.Entry
	if len(detEntryIDs) > 0 {
		if err := e.db.Where("id IN ?", detEntryIDs).Order("timestamp ASC").Find(&detEntries).Error; err != nil {
			log.Printf("incidents: correlate load deterministic entries for %s: %v", dispatch.incidentID, err)
			e.clearPending(dispatch.incidentID)
			return
		}
	}

	scopeNodes := correlationScopeNodes(inc.NodeNames, detEntries, dispatch.nodeName)
	windowStart := inc.OpenedAt.Add(-5 * time.Minute)
	windowEnd := inc.OpenedAt.Add(time.Minute)
	query := e.db.Where(
		"timestamp BETWEEN ? AND ? AND NOT (source = ? AND event IN ?)",
		windowStart,
		windowEnd,
		"webhook",
		[]string{"down", "up"},
	)
	if len(scopeNodes) > 0 {
		query = query.Where("node_name IN ?", scopeNodes)
	}

	var nodeEntries []types.Entry
	if err := query.Order("timestamp DESC").Limit(100).Find(&nodeEntries).Error; err != nil {
		log.Printf("incidents: correlate load node entries for %s: %v", dispatch.incidentID, err)
		e.clearPending(dispatch.incidentID)
		return
	}
	if len(detEntryIDs) > 0 || len(excludedNodeEntryIDs) > 0 {
		existing := make(map[string]struct{}, len(detEntryIDs)+len(excludedNodeEntryIDs))
		for _, entryID := range detEntryIDs {
			existing[entryID] = struct{}{}
		}
		for entryID := range excludedNodeEntryIDs {
			existing[entryID] = struct{}{}
		}
		filtered := nodeEntries[:0]
		for _, entry := range nodeEntries {
			if _, ok := existing[entry.ID]; ok {
				continue
			}
			filtered = append(filtered, entry)
		}
		nodeEntries = filtered
	}

	prompt := buildCorrelationPrompt(inc, detLinks, detEntries, nodeEntries)
	result, err := callCorrelateGenerateFunc(context.Background(), dispatch.provider, dispatch.model, prompt, aiCorrelateTimeout)
	if err != nil {
		log.Printf("incidents: ai correlation failed for %s: %v", dispatch.incidentID, err)
		e.clearPending(dispatch.incidentID)
		return
	}

	var response correlationResponse
	if err := json.Unmarshal([]byte(extractJSON(result)), &response); err != nil {
		log.Printf("incidents: correlate parse response for %s: %v", dispatch.incidentID, err)
		e.clearPending(dispatch.incidentID)
		return
	}

	validIDs := make(map[string]struct{}, len(detEntries)+len(nodeEntries))
	for _, entry := range detEntries {
		validIDs[entry.ID] = struct{}{}
	}
	for _, entry := range nodeEntries {
		validIDs[entry.ID] = struct{}{}
	}

	acceptedCauseCount := 0
	if err := e.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("incident_id = ? AND role = ?", dispatch.incidentID, "ai_cause").Delete(&models.IncidentEntry{}).Error; err != nil {
			return err
		}

		for _, cause := range response.Causes {
			cause.EntryID = strings.TrimSpace(cause.EntryID)
			if cause.EntryID == "" {
				continue
			}
			if _, ok := validIDs[cause.EntryID]; !ok {
				log.Printf("incidents: correlate dropped hallucinated entry_id %s for incident %s", cause.EntryID, dispatch.incidentID)
				continue
			}

			// Skip entries that already have a deterministic link (the delete above
			// removed only ai_cause rows, so any row found here has another role).
			var existing models.IncidentEntry
			err := tx.Where("incident_id = ? AND entry_id = ?", dispatch.incidentID, cause.EntryID).First(&existing).Error
			if err == nil {
				log.Printf("incidents: correlate skipped existing deterministic link %s for incident %s", cause.EntryID, dispatch.incidentID)
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("lookup link %s for incident %s: %w", cause.EntryID, dispatch.incidentID, err)
			}

			score := int(cause.Confidence * 100)
			if score < 0 {
				score = 0
			}
			if score > 100 {
				score = 100
			}

			link := models.IncidentEntry{
				IncidentID: dispatch.incidentID,
				EntryID:    cause.EntryID,
				Role:       "ai_cause",
				Score:      score,
				Reason:     sanitizeExternalText(cause.Reason),
			}
			if err := tx.Create(&link).Error; err != nil {
				return fmt.Errorf("write ai_cause link %s->%s: %w", dispatch.incidentID, cause.EntryID, err)
			}
			acceptedCauseCount++
		}
		return nil
	}); err != nil {
		log.Printf("incidents: correlate write ai_cause links for %s: %v", dispatch.incidentID, err)
		e.clearPending(dispatch.incidentID)
		return
	}

	findings := sanitizeAIFindings(response.Findings)
	annotations := sanitizeAIAnnotations(response.Annotations, validIDs)
	verified := len(response.Causes) == 0
	if response.Verified != nil {
		verified = *response.Verified && acceptedCauseCount == 0
	}

	if !e.updateIncidentMetadata(dispatch.incidentID, func(meta map[string]interface{}) {
		delete(meta, "ai_pending")
		if summary := sanitizeExternalText(response.Summary); summary != "" {
			meta["ai_analysis"] = summary
		}
		if verified {
			meta["ai_verified"] = true
		} else {
			delete(meta, "ai_verified")
		}
		meta["ai_enhanced_ran"] = true
		meta["ai_reviewed_event_count"] = len(detEntries) + len(nodeEntries)
		meta["ai_reviewed_window_start"] = windowStart.UTC().Format(time.RFC3339)
		meta["ai_reviewed_window_end"] = windowEnd.UTC().Format(time.RFC3339)
		if len(findings) > 0 {
			meta["ai_findings"] = findings
		} else {
			delete(meta, "ai_findings")
		}
		if len(annotations) > 0 {
			meta["ai_annotations"] = annotations
		} else {
			delete(meta, "ai_annotations")
		}
		meta["ai_model"] = dispatch.model
		meta["ai_mode"] = dispatch.mode
		meta["ai_enriched_at"] = time.Now().UTC().Format(time.RFC3339)
	}) {
		return
	}
}

func (e *AIEnricher) loadAIConfig() aiConfig {
	keys := []string{aiProviderKey, aiURLKey, aiModelKey, aiAPIKeyKey, aiModeKey, ollamaURLKey, ollamaModelKey, ollamaModeKey}
	var settings []models.AppSetting
	if res := e.db.Where("key IN ?", keys).Find(&settings); res.Error != nil {
		log.Printf("incidents: loadAIConfig: DB query failed: %v", res.Error)
		return aiConfig{mode: "analysis"}
	}

	m := make(map[string]string, len(settings))
	for _, s := range settings {
		m[s.Key] = strings.TrimSpace(s.Value)
	}

	providerType, providerSet := m[aiProviderKey]
	if !providerSet {
		providerType = "ollama"
	}

	aiURL, aiURLSet := m[aiURLKey]
	if !aiURLSet {
		aiURL = m[ollamaURLKey]
	}

	model, modelSet := m[aiModelKey]
	if !modelSet {
		model = m[ollamaModelKey]
	}

	mode, modeSet := m[aiModeKey]
	if !modeSet {
		mode = m[ollamaModeKey]
	}
	if mode == "" {
		mode = "analysis"
	}

	if aiURL == "" || model == "" {
		return aiConfig{mode: mode}
	}

	var provider LLMProvider
	switch providerType {
	case "ollama":
		provider = &ollamaProvider{baseURL: aiURL}
	case "openai_compat":
		provider = &openAICompatProvider{baseURL: aiURL, apiKey: m[aiAPIKeyKey]}
	default:
		log.Printf("incidents: loadAIConfig: unknown ai_provider %q, AI enrichment disabled", providerType)
		return aiConfig{mode: mode}
	}

	return aiConfig{provider: provider, model: model, mode: mode}
}

func (e *AIEnricher) loadAIMode() string {
	return e.loadAIConfig().mode
}

func (e *AIEnricher) setPending(incidentID, model, mode string) bool {
	return e.updateIncidentMetadata(incidentID, func(meta map[string]interface{}) {
		meta["ai_pending"] = true
		meta["ai_model"] = model
		meta["ai_mode"] = mode
		delete(meta, "ai_verified")
	})
}

func (e *AIEnricher) clearPending(incidentID string) {
	e.updateIncidentMetadata(incidentID, func(meta map[string]interface{}) {
		delete(meta, "ai_pending")
	})
}

func (e *AIEnricher) updateIncidentMetadata(incidentID string, apply func(map[string]interface{})) bool {
	var inc models.Incident
	if err := e.db.First(&inc, "id = ?", incidentID).Error; err != nil {
		return false
	}

	meta := make(map[string]interface{})
	if err := json.Unmarshal([]byte(inc.Metadata), &meta); err != nil {
		meta = make(map[string]interface{})
	}
	if meta == nil {
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

func correlationScopeNodes(rawNodeNames string, detEntries []types.Entry, fallbackNode string) []string {
	nodes := preferNonWebhookValues(parseJSONStringSlice(rawNodeNames))
	for _, entry := range detEntries {
		nodes = append(nodes, entry.NodeName)
	}
	nodes = preferNonWebhookValues(nodes)
	fallbackNode = strings.TrimSpace(fallbackNode)
	if len(nodes) == 0 && fallbackNode != "" {
		nodes = append(nodes, fallbackNode)
	}

	return uniqueStrings(nodes)
}

func parseJSONStringSlice(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	return values
}

func preferNonWebhookValues(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}

	nonWebhook := make([]string, 0, len(trimmed))
	for _, value := range trimmed {
		if value != "webhook" {
			nonWebhook = append(nonWebhook, value)
		}
	}
	if len(nonWebhook) > 0 {
		return uniqueStrings(nonWebhook)
	}
	return uniqueStrings(trimmed)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func buildPrompt(inc models.Incident, entries []enrichEntry) string {
	var b strings.Builder
	b.WriteString("You are analyzing a server incident. Provide a concise but useful root cause analysis for an operator.\n")
	b.WriteString("Look for clues that are easy to miss at first glance, especially log snippets, timing, update/config/resource hints, and whether the visible failure is only a symptom.\n\n")
	fmt.Fprintf(&b, "Incident: %s\n", sanitizeExternalText(inc.Title))
	fmt.Fprintf(&b, "Status: %s | Confidence: %s\n\n", inc.Status, inc.Confidence)
	b.WriteString("Events (chronological):\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "- [%s] %s/%s: %s\n", e.Role, e.Source, e.Event, sanitizeExternalText(e.Content))
		if e.Log != "" {
			fmt.Fprintf(&b, "  Log: %s\n", sanitizeExternalText(truncate(e.Log, 300)))
		}
	}
	b.WriteString("\nProvide 3 short sections: 1) Likely cause in 2-3 sentences. 2) Evidence, naming the exact event/log clue. 3) Uncertainty or next check. Do not answer with a single generic sentence.\n")
	return b.String()
}

func writePromptEntry(b *strings.Builder, prefix string, entry types.Entry) {
	fmt.Fprintf(b, "%s[id=%s svc=%s node=%s] %s/%s (%s): %s\n",
		prefix,
		entry.ID,
		entry.Service,
		entry.NodeName,
		entry.Source,
		entry.Event,
		entry.Timestamp.UTC().Format(time.RFC3339),
		sanitizeExternalText(truncate(entry.Content, 200)),
	)
	if logSnippet := extractLogSnippet(&entry); logSnippet != "" {
		fmt.Fprintf(b, "  Log: %s\n", sanitizeExternalText(truncate(logSnippet, 300)))
	}
}

func buildCorrelationPrompt(inc models.Incident, detLinks []models.IncidentEntry, detEntries []types.Entry, nodeEntries []types.Entry) string {
	var b strings.Builder
	b.WriteString("You are analyzing a server incident. Return ONLY valid JSON with no prose or markdown.\n\n")
	fmt.Fprintf(&b, "Incident: %s\n", sanitizeExternalText(inc.Title))
	fmt.Fprintf(&b, "Status: %s | Confidence: %s | Opened: %s\n\n", inc.Status, inc.Confidence, inc.OpenedAt.UTC().Format(time.RFC3339))
	b.WriteString("Goal: find the non-obvious operational clues a human might miss at first glance. Distinguish a system manager symptom from the application, update, config, resource, dependency, or log evidence that likely explains it.\n\n")

	roleByID := make(map[string]string, len(detLinks))
	scoreByID := make(map[string]int, len(detLinks))
	for _, link := range detLinks {
		roleByID[link.EntryID] = link.Role
		scoreByID[link.EntryID] = link.Score
	}

	if len(detEntries) > 0 {
		b.WriteString("Deterministic links already identified by the correlation engine:\n")
		for _, entry := range detEntries {
			fmt.Fprintf(&b, "- [%s score=%d] ",
				roleByID[entry.ID],
				scoreByID[entry.ID],
			)
			writePromptEntry(&b, "", entry)
		}
		b.WriteString("\n")
	}

	b.WriteString("Recent events from the scoped incident timeline. Use entry_id values exactly as written if you identify additional causes:\n")
	for _, entry := range nodeEntries {
		writePromptEntry(&b, "- ", entry)
	}

	b.WriteString(`
Return JSON exactly matching this schema:
{
  "summary": "<2-3 sentence operator-facing conclusion. Name the likely failure, strongest evidence, and uncertainty. Do not be generic.>",
  "verified": true,
  "findings": [
    {
      "kind": "key_clue",
      "confidence": 0.0,
      "title": "<short label for a non-obvious clue>",
      "detail": "<why this clue matters operationally>",
      "evidence": ["<short quoted or paraphrased evidence from event content/logs>"]
    }
  ],
  "annotations": [
    {
      "entry_id": "<verbatim id from any event above>",
      "kind": "key_evidence",
      "confidence": 0.0,
      "title": "<small inline note title>",
      "detail": "<specific interpretation of this existing event or its logs>",
      "evidence": ["<short evidence phrase>"]
    }
  ],
  "causes": [
    {"entry_id": "<verbatim id from the events above>", "confidence": 0.0, "reason": "<why this event caused the incident>"}
  ]
}
Use "findings" for cross-event clues, uncertainty, negative evidence, or log-derived interpretation that should not be crammed into the summary.
Use "annotations" for small notes attached to specific existing events, especially when the key evidence is inside an already-linked trigger/evidence/recovery row.
Use "causes" only for additional timeline events that are not already deterministic links. If an existing deterministic row is important, annotate it instead of repeating it as a cause.
Set "verified" to true when the deterministic links plus annotations explain the incident and no extra AI-only causes are needed.
If no additional causes are found beyond the deterministic links, return an empty causes array.
Keep all titles short and all details concrete. Do not invent entry IDs or evidence.`)

	return b.String()
}

func sanitizeAIFindings(raw []Finding) []aiFindingMetadata {
	findings := make([]aiFindingMetadata, 0, len(raw))
	for _, finding := range raw {
		title := sanitizeExternalText(truncate(strings.TrimSpace(finding.Title), 120))
		detail := sanitizeExternalText(truncate(strings.TrimSpace(finding.Detail), 500))
		if title == "" && detail == "" {
			continue
		}
		findings = append(findings, aiFindingMetadata{
			Kind:       normalizeAIKind(finding.Kind, "finding"),
			Confidence: confidencePercent(finding.Confidence),
			Title:      title,
			Detail:     detail,
			Evidence:   sanitizeEvidenceList(finding.Evidence),
		})
		if len(findings) >= 6 {
			break
		}
	}
	return findings
}

func sanitizeAIAnnotations(raw []Annotation, validIDs map[string]struct{}) []aiAnnotationMetadata {
	annotations := make([]aiAnnotationMetadata, 0, len(raw))
	seenByEntry := make(map[string]int, len(raw))
	for _, annotation := range raw {
		entryID := strings.TrimSpace(annotation.EntryID)
		if entryID == "" {
			continue
		}
		if _, ok := validIDs[entryID]; !ok {
			continue
		}
		if seenByEntry[entryID] >= 2 {
			continue
		}
		title := sanitizeExternalText(truncate(strings.TrimSpace(annotation.Title), 120))
		detail := sanitizeExternalText(truncate(strings.TrimSpace(annotation.Detail), 500))
		if title == "" && detail == "" {
			continue
		}
		annotations = append(annotations, aiAnnotationMetadata{
			EntryID:    entryID,
			Kind:       normalizeAIKind(annotation.Kind, "note"),
			Confidence: confidencePercent(annotation.Confidence),
			Title:      title,
			Detail:     detail,
			Evidence:   sanitizeEvidenceList(annotation.Evidence),
		})
		seenByEntry[entryID]++
		if len(annotations) >= 12 {
			break
		}
	}
	return annotations
}

func sanitizeEvidenceList(values []string) []string {
	evidence := make([]string, 0, len(values))
	for _, value := range values {
		value = sanitizeExternalText(truncate(strings.TrimSpace(value), 180))
		if value == "" {
			continue
		}
		evidence = append(evidence, value)
		if len(evidence) >= 3 {
			break
		}
	}
	return evidence
}

func normalizeAIKind(value, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.NewReplacer(" ", "_", "-", "_").Replace(value)
	if value == "" {
		return fallback
	}
	runes := []rune(value)
	if len(runes) > 40 {
		return string(runes[:40])
	}
	return value
}

func confidencePercent(value float64) int {
	score := int(value * 100)
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		return s
	}
	return s[start : end+1]
}

func callOllamaWithTimeout(ctx context.Context, baseURL, model, prompt string, timeout time.Duration) (string, error) {
	reqBody, _ := json.Marshal(ollamaRequest{Model: model, Prompt: prompt, Stream: false})
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
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
