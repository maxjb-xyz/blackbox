package incidents

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"blackbox/server/internal/correlation"
	"blackbox/server/internal/models"
	"blackbox/server/internal/notify"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

const crashLoopWindow = 5 * time.Minute
const crashLoopThreshold = 3
const suspectedAutoCloseTTL = 10 * time.Minute
const dockerStabilityTTL = 30 * time.Second
const systemdStabilityTTL = 2 * time.Minute
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
	case entry.Source == "systemd" && (entry.Event == "failed" || entry.Event == "oom_kill" || entry.Event == "restart" || entry.Event == "started"):
		m.handleSystemdEvent(entry)
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
			delete(m.pendingRecover, key)
			m.upgradeToConfirmed(incidentID, entry)
			return
		}
	}

	candidates, err := correlation.ScoreCauses(m.db, []string{svc}, entry.Timestamp, entry.ComposeService)
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
	nodeNames := incidentNodeNames(entry.NodeName, candidates)

	incident := models.Incident{
		ID:          incidentID,
		OpenedAt:    entry.Timestamp,
		Status:      "open",
		Confidence:  "confirmed",
		Title:       title,
		Services:    jsonStrings([]string{svc}),
		RootCauseID: rootCauseID,
		TriggerID:   entry.ID,
		NodeNames:   jsonStrings(nodeNames),
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

	m.registerOpenIncidentKeys(incidentID, svc, nodeNames)
	m.broadcastOpened(incident)

	m.dispatchIncidentEnrichment(incidentID, entry.NodeName)
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
	handled := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		incidentID := m.openIncidents[key]
		if _, ok := handled[incidentID]; ok {
			delete(m.openIncidents, key)
			delete(m.pendingRecover, key)
			continue
		}
		handled[incidentID] = struct{}{}
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
		delete(m.pendingRecover, key)
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
		delete(m.pendingRecover, key)
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

	m.openSuspectedIncident(entry, containerExitIncidentReason(exitCode, isCrashLoop))
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
	if inc.Status != "open" {
		return
	}
	if inc.Confidence != "suspected" {
		m.linkEntry(incidentID, entry.ID, "evidence", 0)
		m.broadcastUpdated(incidentID)
		return
	}

	m.pendingRecover[key] = pendingRecovery{
		entry:    entry,
		deadline: entry.Timestamp.Add(dockerStabilityTTL),
	}
}

func (m *Manager) handleWatchtowerUpdate(entry types.Entry) {
	for _, svc := range watchtowerTargetServices(entry) {
		m.pendingWT[svc] = pendingWatchtower{
			entry:    entry,
			deadline: entry.Timestamp.Add(watchtowerPendingTTL),
		}
	}
}

func (m *Manager) handleSystemdEvent(entry types.Entry) {
	switch entry.Event {
	case "started":
		m.handleSystemdStarted(entry)
	default:
		m.handleSystemdInstability(entry)
	}
}

func (m *Manager) handleSystemdInstability(entry types.Entry) {
	svc := entry.Service
	key := incidentKey(svc, entry.NodeName)

	if entry.Event == "failed" || entry.Event == "restart" {
		m.recentSystemdEvents[key] = append(m.recentSystemdEvents[key], entry.Timestamp)
		m.pruneSystemdEvents(key, entry.Timestamp)
	}

	if incidentID, ok := m.openIncidents[key]; ok {
		delete(m.pendingRecover, key)
		m.linkEntry(incidentID, entry.ID, "evidence", 0)
		m.broadcastUpdated(incidentID)
		return
	}

	if entry.Event == "restart" && len(m.recentSystemdEvents[key]) < crashLoopThreshold {
		return
	}

	reason := systemdIncidentReason(entry.Event, len(m.recentSystemdEvents[key]))
	if reason == "" {
		return
	}
	m.openSuspectedIncident(entry, reason)
}

func (m *Manager) handleSystemdStarted(entry types.Entry) {
	key := incidentKey(entry.Service, entry.NodeName)
	incidentID, ok := m.openIncidents[key]
	if !ok {
		return
	}

	var inc models.Incident
	if err := m.db.First(&inc, "id = ?", incidentID).Error; err != nil {
		log.Printf("incidents: load incident %s: %v", incidentID, err)
		return
	}
	if inc.Status != "open" {
		return
	}
	if inc.Confidence != "suspected" {
		m.linkEntry(incidentID, entry.ID, "evidence", 0)
		m.broadcastUpdated(incidentID)
		return
	}

	m.pendingRecover[key] = pendingRecovery{
		entry:    entry,
		deadline: entry.Timestamp.Add(systemdStabilityTTL),
	}
}

func (m *Manager) openSuspectedIncident(trigger types.Entry, reason string) {
	svc := trigger.Service

	candidates, err := correlation.ScoreCauses(m.db, []string{svc}, trigger.Timestamp, trigger.ComposeService)
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
	m.dispatchIncidentEnrichment(incidentID, trigger.NodeName)
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
	m.dispatchIncidentEnrichment(incidentID, trigger.NodeName)
}

func (m *Manager) upgradeToConfirmed(incidentID string, downEntry types.Entry) {
	if err := m.db.Model(&models.IncidentEntry{}).
		Where("incident_id = ? AND role = ?", incidentID, "trigger").
		Update("role", "evidence").Error; err != nil {
		log.Printf("incidents: mark trigger as evidence for %s via %s: %v", incidentID, downEntry.ID, err)
		return
	}

	svc := downEntry.Service
	candidates, err := correlation.ScoreCauses(m.db, []string{svc}, downEntry.Timestamp, downEntry.ComposeService)
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
	if m.notifier != nil {
		var updated models.Incident
		if err := m.db.First(&updated, "id = ?", incidentID).Error; err != nil {
			log.Printf("incidents: reload confirmed incident %s: %v", incidentID, err)
		} else {
			m.notifier.Send(context.Background(), notify.EventIncidentConfirmed, updated)
		}
	}
	m.dispatchIncidentEnrichment(incidentID, downEntry.NodeName)
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
		if m.hasPendingRecovery(inc.ID) {
			continue
		}
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
				delete(m.pendingRecover, key)
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
	sweepExpiredRecoveriesLocked(m, now)
}

func (m *Manager) hasPendingRecovery(incidentID string) bool {
	for key, pending := range m.pendingRecover {
		if pending.entry.ID == "" {
			continue
		}
		if m.openIncidents[key] == incidentID {
			return true
		}
	}
	return false
}

func sweepExpiredPendingWTLocked(m *Manager, now time.Time) {
	for svc, pending := range m.pendingWT {
		if !pending.deadline.After(now) {
			delete(m.pendingWT, svc)
		}
	}
}

func sweepExpiredRecoveriesLocked(m *Manager, now time.Time) {
	for key, pending := range m.pendingRecover {
		if pending.deadline.After(now) {
			continue
		}

		incidentID, ok := m.openIncidents[key]
		if !ok {
			delete(m.pendingRecover, key)
			continue
		}

		var inc models.Incident
		if err := m.db.First(&inc, "id = ?", incidentID).Error; err != nil {
			log.Printf("incidents: load systemd recovery incident %s: %v", incidentID, err)
			delete(m.pendingRecover, key)
			continue
		}
		if inc.Status != "open" {
			delete(m.pendingRecover, key)
			continue
		}

		m.linkEntry(incidentID, pending.entry.ID, "recovery", 0)
		if err := m.db.Model(&models.Incident{}).Where("id = ?", incidentID).Updates(map[string]interface{}{
			"status":      "resolved",
			"resolved_at": pending.deadline,
		}).Error; err != nil {
			log.Printf("incidents: resolve systemd incident %s after stability window: %v", incidentID, err)
			continue
		}

		delete(m.pendingRecover, key)
		delete(m.openIncidents, key)

		var updated models.Incident
		if err := m.db.First(&updated, "id = ?", incidentID).Error; err != nil {
			log.Printf("incidents: reload resolved systemd incident %s: %v", incidentID, err)
			continue
		}
		m.broadcastResolved(updated)
	}
}

func (m *Manager) pruneDies(key string, now time.Time) {
	m.recentDies[key] = pruneRecentTimes(m.recentDies[key], now, crashLoopWindow)
}

func (m *Manager) pruneSystemdEvents(key string, now time.Time) {
	m.recentSystemdEvents[key] = pruneRecentTimes(m.recentSystemdEvents[key], now, crashLoopWindow)
}

func pruneRecentTimes(times []time.Time, now time.Time, window time.Duration) []time.Time {
	cutoff := now.Add(-window)
	// Reusing the backing array in place is intentional: pruneDies and pruneSystemdEvents
	// immediately store the returned slice back into their maps, and no external references
	// to the original slice are kept. If that changes, allocate a new slice here instead.
	filtered := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	return filtered
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

func (m *Manager) dispatchIncidentEnrichment(incidentID, nodeName string) {
	enrichEntries, err := m.enrichmentEntries(incidentID)
	if err != nil {
		log.Printf("incidents: build enrich entries for %s: %v", incidentID, err)
		return
	}
	if len(enrichEntries) == 0 {
		return
	}
	m.DispatchAIAsync(incidentID, enrichEntries, nodeName)
}

func (m *Manager) enrichmentEntries(incidentID string) ([]enrichEntry, error) {
	var links []models.IncidentEntry
	if err := m.db.Where("incident_id = ? AND role <> ?", incidentID, "ai_cause").Find(&links).Error; err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, nil
	}

	roleByID := make(map[string]string, len(links))
	entryIDs := make([]string, 0, len(links))
	for _, link := range links {
		entryID := strings.TrimSpace(link.EntryID)
		if entryID == "" {
			continue
		}
		roleByID[entryID] = link.Role
		entryIDs = append(entryIDs, entryID)
	}
	if len(entryIDs) == 0 {
		return nil, nil
	}

	var entries []types.Entry
	if err := m.db.Where("id IN ?", entryIDs).Find(&entries).Error; err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].Timestamp.Equal(entries[j].Timestamp) {
			return entries[i].Timestamp.Before(entries[j].Timestamp)
		}
		return entries[i].ID < entries[j].ID
	})

	enrichEntries := make([]enrichEntry, 0, len(entries))
	for _, entry := range entries {
		enrichEntry := enrichEntry{
			Role:    roleByID[entry.ID],
			Content: entry.Content,
			Source:  entry.Source,
			Event:   entry.Event,
		}
		if logSnippet := extractLogSnippet(&entry); logSnippet != "" {
			enrichEntry.Log = logSnippet
		}
		enrichEntries = append(enrichEntries, enrichEntry)
	}
	return enrichEntries, nil
}

func (m *Manager) broadcastOpened(inc models.Incident) {
	if m.hub != nil {
		if msg := marshalWSMessage("incident_opened", inc); msg != nil {
			m.hub.Broadcast(msg)
		}
	}
	if m.notifier != nil {
		event := notify.EventIncidentOpenedConfirmed
		if inc.Confidence == "suspected" {
			event = notify.EventIncidentOpenedSuspected
		}
		m.notifier.Send(context.Background(), event, inc)
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
	if m.hub != nil {
		if msg := marshalWSMessage("incident_resolved", inc); msg != nil {
			m.hub.Broadcast(msg)
		}
	}
	if m.notifier != nil {
		m.notifier.Send(context.Background(), notify.EventIncidentResolved, inc)
	}
}

func buildDownTitle(svc string, candidates []correlation.CauseCandidate) string {
	if len(candidates) == 0 {
		return svc + " — monitor down"
	}
	c := candidates[0]
	if c.Entry != nil {
		return fmt.Sprintf("%s — %s %s", svc, c.Entry.Source, c.Entry.Event)
	}
	return svc + " — monitor down"
}

func incidentNodeNames(fallback string, candidates []correlation.CauseCandidate) []string {
	nodes := make([]string, 0, len(candidates)+1)
	seen := make(map[string]struct{}, len(candidates)+1)

	for _, candidate := range candidates {
		if candidate.Entry == nil {
			continue
		}
		node := strings.TrimSpace(candidate.Entry.NodeName)
		if node == "" {
			continue
		}
		if _, ok := seen[node]; ok {
			continue
		}
		seen[node] = struct{}{}
		nodes = append(nodes, node)
	}

	if len(nodes) > 0 {
		return nodes
	}

	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return []string{}
	}
	return []string{fallback}
}

func (m *Manager) registerOpenIncidentKeys(incidentID, service string, nodes []string) {
	if len(nodes) == 0 {
		nodes = []string{""}
	}
	for _, node := range nodes {
		node = strings.TrimSpace(node)
		m.openIncidents[incidentKey(service, node)] = incidentID
	}
}

func jsonStrings(ss []string) string {
	b, err := json.Marshal(ss)
	if err != nil {
		log.Printf("incidents: marshal string slice: %v", err)
		return "[]"
	}
	return string(b)
}

func systemdIncidentReason(event string, instabilityCount int) string {
	if instabilityCount >= crashLoopThreshold {
		return "systemd restart/failure loop"
	}

	switch event {
	case "failed":
		return "systemd unit failed"
	case "oom_kill":
		return "systemd OOM kill"
	default:
		return ""
	}
}

func containerExitIncidentReason(exitCode string, isCrashLoop bool) string {
	exitCode = strings.TrimSpace(exitCode)
	if exitCode != "" && exitCode != "0" {
		return "container exited (exit " + exitCode + ")"
	}
	if isCrashLoop {
		return "container exited repeatedly"
	}
	return "container exited"
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

func watchtowerTargetServices(entry types.Entry) []string {
	services := []string{}
	seen := map[string]struct{}{}

	addService := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		services = append(services, value)
	}

	var meta struct {
		Services []string `json:"watchtower.services"`
	}
	if err := json.Unmarshal([]byte(entry.Metadata), &meta); err == nil {
		for _, service := range meta.Services {
			addService(service)
		}
	}
	if len(services) == 0 {
		addService(entry.Service)
	}
	return services
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
