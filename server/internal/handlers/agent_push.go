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

	isHeartbeat := entry.Source == "agent" && entry.Event == "heartbeat"

	var node models.Node
	result := database.Where("name = ?", entry.NodeName).First(&node)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		newNode := models.Node{
			ID:       ulid.Make().String(),
			Name:     entry.NodeName,
			LastSeen: entry.Timestamp,
		}
		if isHeartbeat {
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

	if !isHeartbeat && entry.Timestamp.Sub(node.LastSeen) < 30*time.Second {
		return
	}

	updates := map[string]interface{}{"last_seen": entry.Timestamp}

	if isHeartbeat {
		var meta struct {
			AgentVersion string `json:"agent_version"`
			IPAddress    string `json:"ip_address"`
			OsInfo       string `json:"os_info"`
		}
		if err := json.Unmarshal([]byte(entry.Metadata), &meta); err == nil {
			if meta.AgentVersion != "" {
				updates["agent_version"] = meta.AgentVersion
			}
			if meta.IPAddress != "" {
				updates["ip_address"] = meta.IPAddress
			}
			if meta.OsInfo != "" {
				updates["os_info"] = meta.OsInfo
			}
		} else {
			log.Printf("upsertNode heartbeat metadata parse error (node=%s): %v", entry.NodeName, err)
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
