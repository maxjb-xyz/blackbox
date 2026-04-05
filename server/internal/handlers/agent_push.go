package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"blackbox/server/internal/hub"
	"blackbox/server/internal/middleware"
	"blackbox/server/internal/models"
	"blackbox/server/internal/services"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

const maxAgentEntryBodyBytes = 64 << 10

func AgentPush(database *gorm.DB, h *hub.Hub, incidentCh chan<- types.Entry) http.HandlerFunc {
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
		serviceName, err := services.NormalizeService(database, entry.Service)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to normalize service")
			return
		}
		if serviceName == "" && !isAgentMetaEvent(entry) {
			writeError(w, http.StatusBadRequest, "service is required")
			return
		}
		entry.Service = serviceName
			if err := database.Create(&entry).Error; err != nil {
				writeError(w, http.StatusInternalServerError, "failed to save entry")
				return
			}
			dispatchToIncidentChannel(incidentCh, entry)
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
	return entry.Source == "agent" && (entry.Event == "heartbeat" || entry.Event == "start")
}

func upsertNode(database *gorm.DB, entry types.Entry) bool {
	if entry.NodeName == "" {
		return false
	}

	now := time.Now().UTC()
	isHeartbeat := entry.Source == "agent" && entry.Event == "heartbeat"
	isStart := entry.Source == "agent" && entry.Event == "start"
	isMetaEvent := isHeartbeat || isStart

	var node models.Node
	result := database.Where("name = ?", entry.NodeName).First(&node)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		newNode := models.Node{
			ID:       ulid.Make().String(),
			Name:     entry.NodeName,
			LastSeen: now,
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
	threshold := time.Now().Add(-7 * time.Minute)
	for _, n := range nodes {
		if n.LastSeen.After(threshold) {
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
