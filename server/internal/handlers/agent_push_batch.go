package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"blackbox/server/internal/hub"
	"blackbox/server/internal/middleware"
	"blackbox/shared/types"
	"gorm.io/gorm"
)

const (
	maxBatchSize           = 200
	maxAgentBatchBodyBytes = 10 << 20 // 10 MB
)

type batchPushResponse struct {
	Accepted []string         `json:"accepted"`
	Failed   []batchPushError `json:"failed"`
}

type batchPushError struct {
	ID        string `json:"id"`
	Reason    string `json:"reason"`
	Permanent bool   `json:"permanent"`
}

// AgentPushBatch accepts a JSON array of entries and ingests each through the
// same validation and upsert logic as AgentPush. Returns 200 with accepted/failed
// lists. Only returns 4xx for structural problems (oversized body, batch > 200).
func AgentPushBatch(database *gorm.DB, h *hub.Hub, incidentCh chan<- types.Entry, shutdown <-chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeName, ok := middleware.AgentNodeFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var entries []types.Entry
		if !decodeJSONBody(w, r, maxAgentBatchBodyBytes, &entries) {
			return
		}

		if len(entries) > maxBatchSize {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("batch too large: max %d entries", maxBatchSize))
			return
		}

		resp := batchPushResponse{
			Accepted: make([]string, 0, len(entries)),
			Failed:   make([]batchPushError, 0),
		}

		nodeUpdated := false

		for _, entry := range entries {
			if entry.ID == "" {
				resp.Failed = append(resp.Failed, batchPushError{ID: entry.ID, Reason: "entry id is required", Permanent: true})
				continue
			}
			if entry.NodeName != "" && entry.NodeName != nodeName {
				resp.Failed = append(resp.Failed, batchPushError{ID: entry.ID, Reason: "agent node mismatch", Permanent: true})
				continue
			}
			entry.NodeName = nodeName
			serviceName := strings.ToLower(strings.TrimSpace(entry.Service))
			if serviceName == "" && !isAgentMetaEvent(entry) {
				resp.Failed = append(resp.Failed, batchPushError{ID: entry.ID, Reason: "service is required", Permanent: true})
				continue
			}
			entry.Service = serviceName

			if entry.Source == "docker" && entry.Event == "restart" {
				var existing types.Entry
				lookupErr := database.First(&existing, "id = ?", entry.ID).Error
				if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
					resp.Failed = append(resp.Failed, batchPushError{ID: entry.ID, Reason: "failed to look up entry"})
					continue
				}
				if lookupErr == nil {
					updates := map[string]interface{}{
						"event":           entry.Event,
						"content":         entry.Content,
						"metadata":        entry.Metadata,
						"timestamp":       entry.Timestamp,
						"compose_service": entry.ComposeService,
					}
					if err := database.Model(&existing).Updates(updates).Error; err != nil {
						resp.Failed = append(resp.Failed, batchPushError{ID: entry.ID, Reason: "failed to update entry"})
						continue
					}
					var updated types.Entry
					if err := database.First(&updated, "id = ?", entry.ID).Error; err != nil {
						resp.Failed = append(resp.Failed, batchPushError{ID: entry.ID, Reason: "failed to fetch updated entry"})
						continue
					}
					if h != nil {
						type replacedPayload struct {
							OldID string      `json:"old_id"`
							Entry types.Entry `json:"entry"`
						}
						if msg := MarshalWSMessage("entry_replaced", replacedPayload{OldID: entry.ID, Entry: updated}); msg != nil {
							h.Broadcast(msg)
						}
					}
					dispatchToIncidentChannelWithShutdown(incidentCh, shutdown, updated)
					if upsertNode(database, updated) {
						nodeUpdated = true
					}
					resp.Accepted = append(resp.Accepted, entry.ID)
					continue
				}
			}

			if err := database.Create(&entry).Error; err != nil {
				resp.Failed = append(resp.Failed, batchPushError{ID: entry.ID, Reason: "failed to save entry"})
				continue
			}
			dispatchToIncidentChannelWithShutdown(incidentCh, shutdown, entry)
			if h != nil {
				if msg := MarshalWSMessage("entry", entry); msg != nil {
					h.Broadcast(msg)
				}
			}
			if upsertNode(database, entry) {
				nodeUpdated = true
			}
			resp.Accepted = append(resp.Accepted, entry.ID)
		}

		if nodeUpdated {
			broadcastNodeStatus(database, h)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			return
		}
	}
}
