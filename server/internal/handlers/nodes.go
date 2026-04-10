package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

type nodeResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	LastSeen     time.Time `json:"last_seen"`
	AgentVersion string    `json:"agent_version"`
	IPAddress    string    `json:"ip_address"`
	OsInfo       string    `json:"os_info"`
	Status       string    `json:"status"`
}

func ListNodes(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var nodes []models.Node
		if err := database.Order("last_seen DESC").Find(&nodes).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch nodes")
			return
		}

		now := time.Now()
		resp := make([]nodeResponse, len(nodes))
		for i, node := range nodes {
			resp[i] = nodeResponse{
				ID:           node.ID,
				Name:         node.Name,
				LastSeen:     node.LastSeen,
				AgentVersion: node.AgentVersion,
				IPAddress:    node.IPAddress,
				OsInfo:       node.OsInfo,
				Status:       effectiveNodeStatus(node, now),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("ListNodes encode error: %v", err)
		}
	}
}
