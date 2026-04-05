package incidents

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"blackbox/server/internal/correlation"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

const crashLoopWindow = 5 * time.Minute
const crashLoopThreshold = 3
const suspectedAutoCloseTTL = 10 * time.Minute
const watchtowerPendingTTL = 5 * time.Minute

func (m *Manager) processEntry(entry types.Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch {
	case entry.Source == "webhook" && entry.Event == "down":
		m.handleMonitorDown(entry)
	case entry.Source == "webhook" && entry.Event == "up":
		m.handleMonitorUp(entry)
	case entry.Source == "docker" && (entry.Event == "die" || entry.Event == "stop"):
		m.handleContainerExit(entry)
	case entry.Source == "docker" && (entry.Event == "restart" || entry.Event == "start"):
		m.handleContainerStart(entry)
	case entry.Source == "webhook" && entry.Event == "update":
		m.handleWatchtowerUpdate(entry)
	}
}

func (m *Manager) handleMonitorDown(entry types.Entry) {
	svc := entry.Service
	prefix := svc + "|"

	// If a suspected incident is already open for this service (any node), upgrade it.
	for key, incidentID := range m.openIncidents {
		if strings.HasPrefix(key, prefix) {
			m.upgradeToConfirmed(incidentID, entry)
			return
		}
	}

	candidates, err := correlation.ScoreCauses(m.db, []string{svc}, entry.Timestamp)
	if err != nil {
		log.Printf("incidents: ScoreCauses error for %s: %v", svc, err)
	}
	candidates = filterCauseCandidates(candidates, excludeEntryIDs(entry.ID))
	correlation.ApplyNodeBonus(candidates, entry.NodeName)

	incidentID := ulid.Make().String()
	title := buildDownTitle(svc, candidates)
	rootCauseID := ""
	if len(candidates) > 0 {
		rootCauseID = candidates[0].Entry.ID
	}

	incident := models.Incident{
		ID:          incidentID,
		OpenedAt:    entry.Timestamp,
		Status:      "open",
		Confidence:  "confirmed",
		Title:       title,
		Services:    jsonStrings([]string{svc}),
		RootCauseID: rootCauseID,
		TriggerID:   entry.ID,
		NodeNames:   jsonStrings([]string{entry.NodeName}),
		Metadata:    "{}",
	}
	if err := m.db.Create(&incident).Error; err != nil {
		log.Printf("incidents: create confirmed incident error: %v", err)
		return
	}

	m.linkEntry(incidentID, entry.ID, "trigger", 0)
	for _, c := range candidates {
		m.linkEntry(incidentID, c.Entry.ID, "cause", c.Score)
	}

	m.openIncidents[incidentKey(svc, entry.NodeName)] = incidentID
	m.broadcastOpened(incident)

	enrichEntries := make([]enrichEntry, 0, len(candidates)+1)
	enrichEntries = append(enrichEntries, enrichEntry{
		Role:    "trigger",
		Content: entry.Content,
		Source:  entry.Source,
		Event:   entry.Event,
	})
	for _, c := range candidates {
		ee := enrichEntry{
			Role:    "cause",
			Content: c.Entry.Content,
			Source:  c.Entry.Source,
			Event:   c.Entry.Event,
		}
		if logSnippet := extractLogSnippet(c.Entry); logSnippet != "" {
			ee.Log = logSnippet
		}
		enrichEntries = append(enrichEntries, ee)
	}
	m.EnrichAsync(incidentID, enrichEntries)
}

func (m *Manager) handleMonitorUp(entry types.Entry) {
	svc := entry.Service
	prefix := svc + "|"

	// Collect all keys for this service (across all nodes).
	var keys []string
	for key := range m.openIncidents {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return
	}

	now := entry.Timestamp
	for _, key := range keys {
		incidentID := m.openIncidents[key]
		m.linkEntry(incidentID, entry.ID, "recovery", 0)
		if err := m.db.Model(&models.Incident{}).
			Where("id = ?", incidentID).
			Updates(map[string]interface{}{
				"status":      "resolved",
				"resolved_at": now,
			}).Error; err != nil {
			log.Printf("incidents: resolve incident error: %v", err)
			continue
		}
		delete(m.openIncidents, key)
		var updated models.Incident
		if err := m.db.First(&updated, "id = ?", incidentID).Error; err != nil {
			log.Printf("incidents: reload resolved incident %s: %v", incidentID, err)
			continue
		}
		m.broadcastResolved(updated)
	}
}

func (m *Manager) handleContainerExit(entry types.Entry) {
	svc := entry.Service
	key := incidentKey(svc, entry.NodeName)

	m.recentDies[key] = append(m.recentDies[key], entry.Timestamp)
	m.pruneDies(key, entry.Timestamp)

	if incidentID, ok := m.openIncidents[key]; ok {
		m.linkEntry(incidentID, entry.ID, "evidence", 0)
		m.broadcastUpdated(incidentID)
		return
	}

	exitCode := extractExitCodeFromEntry(entry)
	isCrashLoop := len(m.recentDies[key]) >= crashLoopThreshold

	if exitCode == "" || exitCode == "0" {
		if !isCrashLoop {
			return
		}
	}

	m.openSuspectedIncident(entry, "container crash (exit "+exitCode+")")
}

func (m *Manager) handleContainerStart(entry types.Entry) {
	svc := entry.Service
	key := incidentKey(svc, entry.NodeName)
	incidentID, ok := m.openIncidents[key]
	if !ok {
		// pendingWT is keyed by service only — watchtower webhooks carry no node info.
		if pw, hasPW := m.pendingWT[svc]; hasPW && entry.Timestamp.Before(pw.deadline) {
			m.openSuspectedIncidentWithCause(entry, pw.entry, "watchtower update triggered restart")
			delete(m.pendingWT, svc)
		}
		return
	}

	var inc models.Incident
	if err := m.db.First(&inc, "id = ?", incidentID).Error; err != nil {
		log.Printf("incidents: load incident %s: %v", incidentID, err)
		return
	}
	if inc.Confidence != "suspected" {
		return
	}

	m.linkEntry(incidentID, entry.ID, "recovery", 0)
	if err := m.db.Model(&models.Incident{}).Where("id = ?", incidentID).Updates(map[string]interface{}{
		"status":      "resolved",
		"resolved_at": entry.Timestamp,
	}).Error; err != nil {
		log.Printf("incidents: resolve suspected incident %s: %v", incidentID, err)
		return
	}

	var updated models.Incident
	if err := m.db.First(&updated, "id = ?", incidentID).Error; err != nil {
		log.Printf("incidents: reload resolved incident %s: %v", incidentID, err)
		return
	}
	delete(m.openIncidents, key)
	m.broadcastResolved(updated)
}

func (m *Manager) handleWatchtowerUpdate(entry types.Entry) {
	svc := entry.Service
	m.pendingWT[svc] = pendingWatchtower{
		entry:    entry,
		deadline: entry.Timestamp.Add(watchtowerPendingTTL),
	}
}

func (m *Manager) openSuspectedIncident(trigger types.Entry, reason string) {
	svc := trigger.Service

	candidates, err := correlation.ScoreCauses(m.db, []string{svc}, trigger.Timestamp)
	if err != nil {
		log.Printf("incidents: ScoreCauses error for %s: %v", svc, err)
	}
	candidates = filterCauseCandidates(candidates, excludeEntryIDs(trigger.ID))
	correlation.ApplyNodeBonus(candidates, trigger.NodeName)

	rootCauseID := ""
	if len(candidates) > 0 {
		rootCauseID = candidates[0].Entry.ID
	}

	incidentID := ulid.Make().String()
	incident := models.Incident{
		ID:          incidentID,
		OpenedAt:    trigger.Timestamp,
		Status:      "open",
		Confidence:  "suspected",
		Title:       svc + " — " + reason,
		Services:    jsonStrings([]string{svc}),
		RootCauseID: rootCauseID,
		TriggerID:   trigger.ID,
		NodeNames:   jsonStrings([]string{trigger.NodeName}),
		Metadata:    "{}",
	}
	if err := m.db.Create(&incident).Error; err != nil {
		log.Printf("incidents: create suspected incident error: %v", err)
		return
	}
	m.linkEntry(incidentID, trigger.ID, "trigger", 0)
	for _, c := range candidates {
		m.linkEntry(incidentID, c.Entry.ID, "cause", c.Score)
	}
	m.openIncidents[incidentKey(svc, trigger.NodeName)] = incidentID
	m.broadcastOpened(incident)
}

func (m *Manager) openSuspectedIncidentWithCause(trigger, cause types.Entry, reason string) {
	svc := trigger.Service
	incidentID := ulid.Make().String()
	incident := models.Incident{
		ID:          incidentID,
		OpenedAt:    trigger.Timestamp,
		Status:      "open",
		Confidence:  "suspected",
		Title:       svc + " — " + reason,
		Services:    jsonStrings([]string{svc}),
		RootCauseID: cause.ID,
		TriggerID:   trigger.ID,
		NodeNames:   jsonStrings([]string{trigger.NodeName}),
		Metadata:    "{}",
	}
	if err := m.db.Create(&incident).Error; err != nil {
		log.Printf("incidents: create suspected incident error: %v", err)
		return
	}
	m.linkEntry(incidentID, trigger.ID, "trigger", 0)
	m.linkEntry(incidentID, cause.ID, "cause", 70)
	m.openIncidents[incidentKey(svc, trigger.NodeName)] = incidentID
	m.broadcastOpened(incident)
}

func (m *Manager) upgradeToConfirmed(incidentID string, downEntry types.Entry) {
	if err := m.db.Model(&models.IncidentEntry{}).
		Where("incident_id = ? AND role = ?", incidentID, "trigger").
		Update("role", "evidence").Error; err != nil {
		log.Printf("incidents: mark trigger as evidence for %s via %s: %v", incidentID, downEntry.ID, err)
		return
	}

	svc := downEntry.Service
	candidates, err := correlation.ScoreCauses(m.db, []string{svc}, downEntry.Timestamp)
	if err != nil {
		log.Printf("incidents: ScoreCauses while upgrading %s via %s: %v", incidentID, downEntry.ID, err)
	}
	candidates = filterCauseCandidates(candidates, m.linkedEntryIDs(incidentID, downEntry.ID))
	correlation.ApplyNodeBonus(candidates, downEntry.NodeName)

	rootCauseID := ""
	if len(candidates) > 0 {
		rootCauseID = candidates[0].Entry.ID
	}

	updates := map[string]interface{}{
		"confidence": "confirmed",
		"trigger_id": downEntry.ID,
	}
	if rootCauseID != "" {
		updates["root_cause_id"] = rootCauseID
	}
	if err := m.db.Model(&models.Incident{}).Where("id = ?", incidentID).Updates(updates).Error; err != nil {
		log.Printf("incidents: update incident %s while upgrading via %s: %v", incidentID, downEntry.ID, err)
		return
	}
	m.linkEntry(incidentID, downEntry.ID, "trigger", 0)
	for _, c := range candidates {
		m.linkEntry(incidentID, c.Entry.ID, "cause", c.Score)
	}

	m.broadcastUpdated(incidentID)
}

func sweepExpiredSuspectedLocked(m *Manager) {
	now := time.Now().UTC()
	cutoff := now.Add(-suspectedAutoCloseTTL)
	var stale []models.Incident
	if err := m.db.Where("status = ? AND confidence = ? AND opened_at < ?", "open", "suspected", cutoff).Find(&stale).Error; err != nil {
		log.Printf("incidents: load stale suspected incidents: %v", err)
		return
	}
	for _, inc := range stale {
		meta := map[string]interface{}{"auto_closed": true}
		metaBytes, err := json.Marshal(meta)
		if err != nil {
			log.Printf("incidents: encode auto-close metadata for %s: %v", inc.ID, err)
			continue
		}
		result := m.db.Model(&models.Incident{}).Where("id = ?", inc.ID).Updates(map[string]interface{}{
			"status":      "resolved",
			"resolved_at": now,
			"metadata":    string(metaBytes),
		})
		if result.Error != nil {
			log.Printf("incidents: auto-resolve suspected incident %s: %v", inc.ID, result.Error)
			continue
		}
		if result.RowsAffected == 0 {
			log.Printf("incidents: auto-resolve suspected incident %s: no rows updated", inc.ID)
			continue
		}
		for key, id := range m.openIncidents {
			if id == inc.ID {
				delete(m.openIncidents, key)
				break
			}
		}
		var updated models.Incident
		if err := m.db.First(&updated, "id = ?", inc.ID).Error; err != nil {
			log.Printf("incidents: reload auto-resolved incident %s: %v", inc.ID, err)
			continue
		}
		m.broadcastResolved(updated)
	}
	sweepExpiredPendingWTLocked(m, now)
}

func sweepExpiredPendingWTLocked(m *Manager, now time.Time) {
	for svc, pending := range m.pendingWT {
		if !pending.deadline.After(now) {
			delete(m.pendingWT, svc)
		}
	}
}

func (m *Manager) pruneDies(key string, now time.Time) {
	cutoff := now.Add(-crashLoopWindow)
	filtered := m.recentDies[key][:0]
	for _, t := range m.recentDies[key] {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	m.recentDies[key] = filtered
}

func (m *Manager) linkEntry(incidentID, entryID, role string, score int) {
	var existing models.IncidentEntry
	err := m.db.Where("incident_id = ? AND entry_id = ?", incidentID, entryID).First(&existing).Error
	if err == nil {
		return
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		log.Printf("incidents: lookup incident link %s -> %s: %v", entryID, incidentID, err)
		return
	}

	ie := models.IncidentEntry{
		IncidentID: incidentID,
		EntryID:    entryID,
		Role:       role,
		Score:      score,
	}
	if err := m.db.Create(&ie).Error; err != nil {
		log.Printf("incidents: link entry %s -> %s: %v", entryID, incidentID, err)
	}
}

func filterCauseCandidates(candidates []correlation.CauseCandidate, excluded map[string]struct{}) []correlation.CauseCandidate {
	if len(candidates) == 0 || len(excluded) == 0 {
		return candidates
	}

	filtered := make([]correlation.CauseCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Entry == nil {
			continue
		}
		if _, skip := excluded[candidate.Entry.ID]; skip {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func excludeEntryIDs(ids ...string) map[string]struct{} {
	excluded := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		excluded[id] = struct{}{}
	}
	return excluded
}

func (m *Manager) linkedEntryIDs(incidentID string, extraIDs ...string) map[string]struct{} {
	excluded := excludeEntryIDs(extraIDs...)

	var links []models.IncidentEntry
	if err := m.db.Where("incident_id = ?", incidentID).Find(&links).Error; err != nil {
		log.Printf("incidents: load links for %s: %v", incidentID, err)
		return excluded
	}
	for _, link := range links {
		if link.EntryID == "" {
			continue
		}
		excluded[link.EntryID] = struct{}{}
	}
	return excluded
}

func (m *Manager) broadcastOpened(inc models.Incident) {
	if m.hub == nil {
		return
	}
	if msg := marshalWSMessage("incident_opened", inc); msg != nil {
		m.hub.Broadcast(msg)
	}
}

func (m *Manager) broadcastUpdated(incidentID string) {
	if m.hub == nil {
		return
	}
	var inc models.Incident
	if err := m.db.First(&inc, "id = ?", incidentID).Error; err != nil {
		return
	}
	if msg := marshalWSMessage("incident_updated", inc); msg != nil {
		m.hub.Broadcast(msg)
	}
}

func (m *Manager) broadcastResolved(inc models.Incident) {
	if m.hub == nil {
		return
	}
	if msg := marshalWSMessage("incident_resolved", inc); msg != nil {
		m.hub.Broadcast(msg)
	}
}

func buildDownTitle(svc string, candidates []correlation.CauseCandidate) string {
	if len(candidates) == 0 {
		return svc + " — monitor down"
	}
	return svc + " — " + candidates[0].Reason
}

func jsonStrings(ss []string) string {
	b, err := json.Marshal(ss)
	if err != nil {
		log.Printf("incidents: marshal string slice: %v", err)
		return "[]"
	}
	return string(b)
}

func extractExitCodeFromEntry(e types.Entry) string {
	var meta map[string]json.RawMessage
	if err := json.Unmarshal([]byte(e.Metadata), &meta); err != nil {
		return ""
	}
	if raw, ok := meta["exitCode"]; ok {
		var code string
		if err := json.Unmarshal(raw, &code); err == nil {
			return code
		}
		var numeric int
		if err := json.Unmarshal(raw, &numeric); err == nil {
			return strconv.Itoa(numeric)
		}
	}
	if rawEventsRaw, ok := meta["raw_events"]; ok {
		var rawEvents []struct {
			Attributes map[string]string `json:"attributes"`
		}
		if err := json.Unmarshal(rawEventsRaw, &rawEvents); err == nil {
			for _, re := range rawEvents {
				if code := re.Attributes["exitCode"]; code != "" {
					return code
				}
			}
		}
	}
	return ""
}

func extractLogSnippet(e *types.Entry) string {
	if e == nil {
		return ""
	}

	var meta struct {
		LogSnippet []string `json:"log_snippet"`
	}
	if err := json.Unmarshal([]byte(e.Metadata), &meta); err != nil || len(meta.LogSnippet) == 0 {
		return ""
	}
	return strings.Join(meta.LogSnippet, "\n")
}

// marshalWSMessage mirrors handlers.MarshalWSMessage to avoid a circular import.
func marshalWSMessage(msgType string, data interface{}) []byte {
	type wsMsg struct {
		Type string      `json:"type"`
		Data interface{} `json:"data"`
	}
	b, err := json.Marshal(wsMsg{Type: msgType, Data: data})
	if err != nil {
		return nil
	}
	return b
}
