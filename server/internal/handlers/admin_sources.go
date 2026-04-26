package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

// SourceTypeDef describes a built-in source type for the catalog UI.
type SourceTypeDef struct {
	Type        string `json:"type"`
	Scope       string `json:"scope"`
	Singleton   bool   `json:"singleton"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Mechanism   string `json:"mechanism"`
}

var knownSourceTypes = []SourceTypeDef{
	{Type: "docker", Scope: "agent", Singleton: true, Name: "Docker", Description: "Container lifecycle events from the local Docker socket", Mechanism: "agent · socket"},
	{Type: "systemd", Scope: "agent", Singleton: true, Name: "Systemd", Description: "Service state changes for watched units via journald", Mechanism: "agent · journal"},
	{Type: "filewatcher", Scope: "agent", Singleton: true, Name: "File Watcher", Description: "inotify events on watched config paths", Mechanism: "agent · inotify"},
	{Type: "proxmox", Scope: "agent", Singleton: false, Name: "Proxmox VE", Description: "VM and container task events polled from the Proxmox API", Mechanism: "agent · poll"},
	{Type: "webhook_uptime_kuma", Scope: "server", Singleton: true, Name: "Uptime Kuma", Description: "Inbound webhook for Uptime Kuma monitor events", Mechanism: "server · http"},
	{Type: "webhook_watchtower", Scope: "server", Singleton: true, Name: "Watchtower", Description: "Inbound webhook for Watchtower container update events", Mechanism: "server · http"},
}

var knownTypes = func() map[string]SourceTypeDef {
	m := make(map[string]SourceTypeDef, len(knownSourceTypes))
	for _, t := range knownSourceTypes {
		m[t.Type] = t
	}
	return m
}()

type sourcesResponse struct {
	Server []models.DataSourceInstance    `json:"server"`
	Nodes  map[string]nodeSourcesResponse `json:"nodes"`
}

type nodeSourcesResponse struct {
	Capabilities []string                    `json:"capabilities"`
	AgentVersion string                      `json:"agent_version"`
	Status       string                      `json:"status"`
	Sources      []models.DataSourceInstance `json:"sources"`
}

func ListSources(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var instances []models.DataSourceInstance
		if err := db.Order("created_at ASC").Find(&instances).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list sources")
			return
		}
		var nodes []models.Node
		if err := db.Find(&nodes).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list nodes")
			return
		}

		now := time.Now()
		resp := sourcesResponse{
			Server: []models.DataSourceInstance{},
			Nodes:  map[string]nodeSourcesResponse{},
		}

		for _, n := range nodes {
			var caps []string
			if err := json.Unmarshal([]byte(n.Capabilities), &caps); err != nil {
				caps = []string{"docker", "systemd", "filewatcher"}
			}
			resp.Nodes[n.Name] = nodeSourcesResponse{
				Capabilities: caps,
				AgentVersion: n.AgentVersion,
				Status:       effectiveNodeStatus(n, now),
				Sources:      []models.DataSourceInstance{},
			}
		}

		for _, inst := range instances {
			if inst.Scope == "server" {
				resp.Server = append(resp.Server, inst)
			} else if inst.NodeID != nil {
				if nr, ok := resp.Nodes[*inst.NodeID]; ok {
					nr.Sources = append(nr.Sources, inst)
					resp.Nodes[*inst.NodeID] = nr
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("ListSources encode: %v", err)
		}
	}
}

func ListSourceTypes() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(knownSourceTypes); err != nil {
			log.Printf("ListSourceTypes encode: %v", err)
		}
	}
}

type createSourceRequest struct {
	Type    string          `json:"type"`
	Scope   string          `json:"scope"`
	NodeID  *string         `json:"node_id"`
	Name    string          `json:"name"`
	Config  json.RawMessage `json:"config"`
	Enabled *bool           `json:"enabled"`
}

func CreateSource(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createSourceRequest
		if !decodeJSONBody(w, r, 64<<10, &req) {
			return
		}
		if _, ok := knownTypes[req.Type]; !ok {
			writeError(w, http.StatusBadRequest, "unknown source type: "+req.Type)
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		cfg := "{}"
		if len(req.Config) > 0 {
			cfg = string(req.Config)
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		now := time.Now().UTC()
		inst := models.DataSourceInstance{
			ID: ulid.Make().String(), Type: req.Type, Scope: req.Scope,
			NodeID: req.NodeID, Name: req.Name, Config: cfg,
			Enabled: enabled, CreatedAt: now, UpdatedAt: now,
		}
		if err := db.Create(&inst).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create source")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(inst); err != nil {
			log.Printf("CreateSource encode: %v", err)
		}
	}
}

type updateSourceRequest struct {
	Name    *string         `json:"name"`
	Config  json.RawMessage `json:"config"`
	Enabled *bool           `json:"enabled"`
}

func UpdateSource(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var inst models.DataSourceInstance
		if err := db.First(&inst, "id = ?", id).Error; err != nil {
			writeError(w, http.StatusNotFound, "source not found")
			return
		}
		var req updateSourceRequest
		if !decodeJSONBody(w, r, 64<<10, &req) {
			return
		}
		if req.Name != nil {
			inst.Name = *req.Name
		}
		if len(req.Config) > 0 {
			inst.Config = string(req.Config)
		}
		if req.Enabled != nil {
			inst.Enabled = *req.Enabled
		}
		inst.UpdatedAt = time.Now().UTC()
		if err := db.Save(&inst).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update source")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(inst); err != nil {
			log.Printf("UpdateSource encode: %v", err)
		}
	}
}

func DeleteSource(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		result := db.Delete(&models.DataSourceInstance{}, "id = ?", id)
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete source")
			return
		}
		if result.RowsAffected == 0 {
			writeError(w, http.StatusNotFound, "source not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetWebhookSecret returns the secret for a webhook source type.
// Falls back to envFallback if the DB instance has no secret set.
func GetWebhookSecret(db *gorm.DB, sourceType, envFallback string) string {
	var inst models.DataSourceInstance
	if err := db.Where("type = ? AND enabled = ?", sourceType, true).First(&inst).Error; err != nil {
		return envFallback
	}
	var cfg struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal([]byte(inst.Config), &cfg); err != nil || cfg.Secret == "" {
		return envFallback
	}
	return cfg.Secret
}
