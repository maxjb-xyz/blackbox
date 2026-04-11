package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/hub"
	"blackbox/server/internal/middleware"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

const maxAgentEntryBodyBytes = 64 << 10

const (
	nodeStatusOnline    = "online"
	nodeStatusOffline   = "offline"
	nodeOfflineAfter    = 7 * time.Minute
	nodeStatusPollEvery = 30 * time.Second
)

func AgentPush(database *gorm.DB, h *hub.Hub, incidentCh chan<- types.Entry, shutdown <-chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeName, ok := middleware.AgentNodeFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var entry types.Entry
		if !decodeJSONBody(w, r, maxAgentEntryBodyBytes, &entry) {
			return
		}
		if entry.ID == "" {
			writeError(w, http.StatusBadRequest, "entry id is required")
			return
		}
		if entry.NodeName != "" && entry.NodeName != nodeName {
			writeError(w, http.StatusForbidden, "agent node mismatch")
			return
		}
		entry.NodeName = nodeName
		serviceName := strings.ToLower(strings.TrimSpace(entry.Service))
		if serviceName == "" && !isAgentMetaEvent(entry) {
			writeError(w, http.StatusBadRequest, "service is required")
			return
		}
		entry.Service = serviceName
		if entry.Source == "docker" && entry.Event == "restart" {
			// ReplaceID carries the stop entry's ID when the agent uses the
			// persistent queue (restart has its own fresh ID). Fall back to
			// entry.ID for entries sent via the legacy single-push path.
			lookupID := entry.ID
			if entry.ReplaceID != "" {
				lookupID = entry.ReplaceID
			}
			var existing types.Entry
			lookupErr := database.First(&existing, "id = ? AND node_name = ? AND source = ?", lookupID, nodeName, "docker").Error
			if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusInternalServerError, "failed to look up entry")
				return
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
					writeError(w, http.StatusInternalServerError, "failed to update entry")
					return
				}
				if err := database.Take(&existing, "id = ? AND node_name = ? AND source = ?", lookupID, nodeName, "docker").Error; err != nil {
					writeError(w, http.StatusInternalServerError, "failed to fetch updated entry")
					return
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
				if upsertNode(database, existing) {
					broadcastNodeStatus(database, h)
				}
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		if err := database.Create(&entry).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save entry")
			return
		}
		dispatchToIncidentChannelWithShutdown(incidentCh, shutdown, entry)
		if h != nil {
			if msg := MarshalWSMessage("entry", entry); msg != nil {
				h.Broadcast(msg)
			}
		}
		if upsertNode(database, entry) {
			broadcastNodeStatus(database, h)
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func isAgentMetaEvent(entry types.Entry) bool {
	return entry.Source == "agent" && (entry.Event == "heartbeat" || entry.Event == "start" || entry.Event == "shutdown")
}

func upsertNode(database *gorm.DB, entry types.Entry) bool {
	if entry.NodeName == "" {
		return false
	}

	now := time.Now().UTC()
	isMetaEvent := isAgentMetaEvent(entry)

	var node models.Node
	result := database.Where("name = ?", entry.NodeName).First(&node)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		newNode := models.Node{
			ID:       ulid.Make().String(),
			Name:     entry.NodeName,
			LastSeen: now,
			Status:   statusForEntry(entry),
		}
		if isMetaEvent {
			applyHeartbeatMeta(&newNode, entry.Metadata)
		}
		if err := database.Create(&newNode).Error; err != nil {
			log.Printf("upsertNode create error (node=%s): %v", entry.NodeName, err)
			return false
		}
		return true
	}

	if result.Error != nil {
		log.Printf("upsertNode lookup error (node=%s): %v", entry.NodeName, result.Error)
		return false
	}

	if !isMetaEvent && now.Sub(node.LastSeen) < 30*time.Second {
		return false
	}

	updates := map[string]interface{}{"last_seen": now}

	if isMetaEvent {
		updates["status"] = statusForEntry(entry)
		updatedNode := node
		applyHeartbeatMeta(&updatedNode, entry.Metadata)
		if updatedNode.AgentVersion != "" {
			updates["agent_version"] = updatedNode.AgentVersion
		}
		if updatedNode.IPAddress != "" {
			updates["ip_address"] = updatedNode.IPAddress
		}
		if updatedNode.OsInfo != "" {
			updates["os_info"] = updatedNode.OsInfo
		}
	}

	if err := database.Model(&node).Updates(updates).Error; err != nil {
		log.Printf("upsertNode update error (node=%s): %v", entry.NodeName, err)
		return false
	}
	return true
}

func applyHeartbeatMeta(node *models.Node, metadata string) {
	var meta struct {
		AgentVersion string `json:"agent_version"`
		IPAddress    string `json:"ip_address"`
		OsInfo       string `json:"os_info"`
	}
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		return
	}
	if meta.AgentVersion != "" {
		node.AgentVersion = meta.AgentVersion
	}
	if meta.IPAddress != "" {
		node.IPAddress = meta.IPAddress
	}
	if meta.OsInfo != "" {
		node.OsInfo = meta.OsInfo
	}
}

func broadcastNodeStatus(database *gorm.DB, h *hub.Hub) {
	if h == nil {
		return
	}
	var nodes []models.Node
	if err := database.Find(&nodes).Error; err != nil {
		return
	}
	online := 0
	now := time.Now()
	for _, n := range nodes {
		if effectiveNodeStatus(n, now) == nodeStatusOnline {
			online++
		}
	}
	type nodeStatus struct {
		Online int `json:"online"`
		Total  int `json:"total"`
	}
	if msg := MarshalWSMessage("node_status", nodeStatus{Online: online, Total: len(nodes)}); msg != nil {
		h.Broadcast(msg)
	}
}

func StartNodeStatusMonitor(ctx context.Context, database *gorm.DB, h *hub.Hub, interval time.Duration) {
	if ctx == nil || database == nil {
		return
	}
	if interval <= 0 {
		interval = nodeStatusPollEvery
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		run := func() {
			changed, err := reconcileNodeStatuses(database, time.Now().UTC())
			if err != nil {
				log.Printf("node-status-monitor: reconcile failed: %v", err)
				return
			}
			if changed {
				broadcastNodeStatus(database, h)
			}
		}

		run()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func reconcileNodeStatuses(database *gorm.DB, now time.Time) (bool, error) {
	tx := database.Begin()
	if tx.Error != nil {
		return false, tx.Error
	}

	var nodes []models.Node
	if err := tx.Find(&nodes).Error; err != nil {
		tx.Rollback()
		return false, err
	}

	changed := false
	for _, node := range nodes {
		if !needsOfflineTransition(node, now) {
			continue
		}
		if err := tx.Model(&models.Node{}).Where("id = ?", node.ID).Update("status", nodeStatusOffline).Error; err != nil {
			tx.Rollback()
			return false, err
		}
		changed = true
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return false, err
	}

	return changed, nil
}

func needsOfflineTransition(node models.Node, now time.Time) bool {
	return node.Status != nodeStatusOffline && effectiveNodeStatus(node, now) == nodeStatusOffline
}

func statusForEntry(entry types.Entry) string {
	if entry.Source == "agent" && entry.Event == "shutdown" {
		return nodeStatusOffline
	}
	return nodeStatusOnline
}

func effectiveNodeStatus(node models.Node, now time.Time) string {
	if node.Status == nodeStatusOffline {
		return nodeStatusOffline
	}
	if now.Sub(node.LastSeen) > nodeOfflineAfter {
		return nodeStatusOffline
	}
	return nodeStatusOnline
}
