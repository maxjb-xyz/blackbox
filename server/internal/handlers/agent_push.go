package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

func AgentPush(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var entry types.Entry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if entry.ID == "" {
			writeError(w, http.StatusBadRequest, "entry id is required")
			return
		}
		if err := database.Create(&entry).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save entry")
			return
		}
		upsertNode(database, entry)
		w.WriteHeader(http.StatusCreated)
	}
}

func upsertNode(database *gorm.DB, entry types.Entry) {
	if entry.NodeName == "" {
		return
	}

	now := time.Now().UTC()
	isHeartbeat := entry.Source == "agent" && entry.Event == "heartbeat"
	isStart := entry.Event == "start"
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
		}
		return
	}

	if result.Error != nil {
		log.Printf("upsertNode lookup error (node=%s): %v", entry.NodeName, result.Error)
		return
	}

	if !isMetaEvent && now.Sub(node.LastSeen) < 30*time.Second {
		return
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
	}
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
