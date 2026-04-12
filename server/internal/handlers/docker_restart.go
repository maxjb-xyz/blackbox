package handlers

import (
	"errors"

	"blackbox/server/internal/hub"
	"blackbox/shared/types"
	"gorm.io/gorm"
)

// replaceDockerRestartResult is returned by replaceDockerRestartEntry.
type replaceDockerRestartResult struct {
	Updated types.Entry
	Found   bool
	Err     error
}

// replaceDockerRestartEntry looks up the stop entry that a docker restart
// supersedes, applies the restart entry's fields to it, re-fetches the updated
// row, and broadcasts the replacement over the WebSocket hub.
//
// The lookup is scoped to nodeName and source="docker" so a client-supplied
// ReplaceID cannot reference entries belonging to other nodes or sources.
//
// Return semantics:
//   - Found=false, Err=nil  → no matching stop found; caller should insert as new
//   - Found=true,  Err=nil  → update applied; Updated holds the refreshed row
//   - Found=false, Err!=nil → a DB error occurred; caller should surface it
func replaceDockerRestartEntry(
	database *gorm.DB,
	entry types.Entry,
	nodeName string,
	h *hub.Hub,
	incidentCh chan<- types.Entry,
	shutdown <-chan struct{},
) replaceDockerRestartResult {
	lookupID := entry.ID
	if entry.ReplaceID != "" {
		lookupID = entry.ReplaceID
	}

	var existing types.Entry
	lookupErr := database.First(&existing, "id = ? AND node_name = ? AND source = ? AND event IN ?", lookupID, nodeName, "docker", []string{"stop", "restart"}).Error
	if lookupErr != nil {
		if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return replaceDockerRestartResult{Found: false}
		}
		return replaceDockerRestartResult{Err: lookupErr}
	}

	updates := map[string]interface{}{
		"event":           entry.Event,
		"content":         entry.Content,
		"metadata":        entry.Metadata,
		"timestamp":       entry.Timestamp,
		"compose_service": entry.ComposeService,
	}
	if err := database.Model(&existing).Updates(updates).Error; err != nil {
		return replaceDockerRestartResult{Err: err}
	}
	// Refresh to pick up server-side defaults written during the update.
	if err := database.Take(&existing, "id = ? AND node_name = ? AND source = ?", lookupID, nodeName, "docker").Error; err != nil {
		return replaceDockerRestartResult{Err: err}
	}

	if h != nil {
		type replacedPayload struct {
			OldID string      `json:"old_id"`
			Entry types.Entry `json:"entry"`
		}
		if msg := MarshalWSMessage("entry_replaced", replacedPayload{OldID: existing.ID, Entry: existing}); msg != nil {
			h.Broadcast(msg)
		}
	}
	dispatchToIncidentChannelWithShutdown(incidentCh, shutdown, existing)

	return replaceDockerRestartResult{Updated: existing, Found: true}
}
