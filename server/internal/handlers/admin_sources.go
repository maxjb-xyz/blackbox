package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
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
	Server  []models.DataSourceInstance    `json:"server"`
	Nodes   map[string]nodeSourcesResponse `json:"nodes"`
	Orphans []models.DataSourceInstance    `json:"orphans"`
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
			Server:  []models.DataSourceInstance{},
			Nodes:   map[string]nodeSourcesResponse{},
			Orphans: []models.DataSourceInstance{},
		}

		for _, n := range nodes {
			var caps []string
			if err := json.Unmarshal([]byte(n.Capabilities), &caps); err != nil {
				log.Printf("ListSources: node %s has invalid capabilities JSON: %v", n.Name, err)
				caps = []string{"docker", "systemd", "filewatcher"}
			} else if len(caps) == 0 {
				log.Printf("ListSources: node %s has no stored capabilities yet; using legacy fallback", n.Name)
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
			redacted := inst
			redacted.Config = redactConfig(inst.Type, inst.Config)

			if inst.Scope == "server" {
				resp.Server = append(resp.Server, redacted)
				continue
			}

			if inst.NodeID != nil {
				if nr, ok := resp.Nodes[*inst.NodeID]; ok {
					nr.Sources = append(nr.Sources, redacted)
					resp.Nodes[*inst.NodeID] = nr
					continue
				}
			}

			resp.Orphans = append(resp.Orphans, redacted)
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
		typeDef, ok := knownTypes[req.Type]
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown source type: "+req.Type)
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}

		// Enforce scope from type definition (not caller-supplied)
		scope := typeDef.Scope

		// Enforce node_id: agent-scoped sources require it; server-scoped sources must not have it
		if scope == "agent" && (req.NodeID == nil || *req.NodeID == "") {
			writeError(w, http.StatusBadRequest, "node_id is required for agent-scoped sources")
			return
		}
		if scope == "server" && req.NodeID != nil && *req.NodeID != "" {
			writeError(w, http.StatusBadRequest, "node_id must not be set for server-scoped sources")
			return
		}

		// Enforce singleton: only one instance per type per node (or per server)
		if typeDef.Singleton {
			var count int64
			q := db.Model(&models.DataSourceInstance{}).Where("type = ?", req.Type)
			if scope == "agent" {
				q = q.Where("node_id = ?", *req.NodeID)
			}
			if err := q.Count(&count).Error; err != nil {
				writeError(w, http.StatusInternalServerError, "failed to check existing sources")
				return
			}
			if count > 0 {
				writeError(w, http.StatusConflict, "a source of this type already exists for this target")
				return
			}
		}

		cfg := "{}"
		if len(req.Config) > 0 {
			// Validate it's a JSON object
			var obj map[string]any
			if err := json.Unmarshal(req.Config, &obj); err != nil {
				writeError(w, http.StatusBadRequest, "config must be a JSON object")
				return
			}
			if obj == nil {
				writeError(w, http.StatusBadRequest, "config must be a JSON object")
				return
			}
			cfg = string(req.Config)
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		now := time.Now().UTC()
		inst := models.DataSourceInstance{
			ID: ulid.Make().String(), Type: req.Type, Scope: scope,
			NodeID: req.NodeID, Name: req.Name, Config: cfg,
			Enabled: enabled, CreatedAt: now, UpdatedAt: now,
		}
		if err := db.Create(&inst).Error; err != nil {
			if isDuplicateKeyError(err) {
				writeError(w, http.StatusConflict, "a source of this type already exists for this target")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to create source")
			return
		}
		refreshWebhookSecretCacheIfNeeded(db, req.Type)
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
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusNotFound, "source not found")
				return
			}
			log.Printf("UpdateSource: lookup failed for %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "database error")
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
			mergedConfig, err := mergeSourceConfig(inst.Type, inst.Config, req.Config)
			if err != nil {
				writeError(w, http.StatusBadRequest, "config must be a JSON object")
				return
			}
			inst.Config = mergedConfig
		}
		if req.Enabled != nil {
			inst.Enabled = *req.Enabled
		}
		inst.UpdatedAt = time.Now().UTC()
		if err := db.Save(&inst).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update source")
			return
		}
		refreshWebhookSecretCacheIfNeeded(db, inst.Type)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(inst); err != nil {
			log.Printf("UpdateSource encode: %v", err)
		}
	}
}

func DeleteSource(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var inst models.DataSourceInstance
		if err := db.First(&inst, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusNotFound, "source not found")
				return
			}
			log.Printf("DeleteSource: lookup failed for %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		result := db.Delete(&models.DataSourceInstance{}, "id = ?", id)
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete source")
			return
		}
		refreshWebhookSecretCacheIfNeeded(db, inst.Type)
		w.WriteHeader(http.StatusNoContent)
	}
}

// redactConfig removes sensitive fields from source config before sending to clients.
func redactConfig(sourceType, config string) string {
	keys := sensitiveKeysFor(sourceType)
	if len(keys) == 0 {
		return config
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(config), &m); err != nil {
		return "{}"
	}
	for _, key := range keys {
		if _, ok := m[key]; ok {
			m[key] = ""
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		log.Printf("redactConfig: failed to marshal redacted %s config: %v", sourceType, err)
		return "{}"
	}
	return string(out)
}

// GetWebhookSecret returns the secret for a webhook source type.
// Falls back to envFallback if the DB instance has no secret set.
func GetWebhookSecret(db *gorm.DB, sourceType, envFallback string) string {
	var inst models.DataSourceInstance
	if err := db.Where("type = ? AND enabled = ?", sourceType, true).Order("created_at ASC").First(&inst).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return envFallback
		}
		log.Printf("GetWebhookSecret: sourceType=%s lookup failed: %v", sourceType, err)
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

func sensitiveKeysFor(sourceType string) []string {
	switch {
	case strings.HasPrefix(sourceType, "webhook_"):
		return []string{"secret"}
	default:
		return nil
	}
}

func mergeSourceConfig(sourceType, existingConfig string, incomingRaw json.RawMessage) (string, error) {
	var incoming map[string]any
	if err := json.Unmarshal(incomingRaw, &incoming); err != nil {
		return "", err
	}
	if incoming == nil {
		return "", errors.New("config must be a JSON object")
	}

	existing := map[string]any{}
	if existingConfig != "" {
		if err := json.Unmarshal([]byte(existingConfig), &existing); err != nil {
			existing = map[string]any{}
		}
	}

	for key, value := range existing {
		if _, ok := incoming[key]; !ok {
			incoming[key] = value
		}
	}

	for _, key := range sensitiveKeysFor(sourceType) {
		incomingValue, ok := incoming[key]
		if !ok {
			continue
		}
		existingValue, existingOK := existing[key]
		if !existingOK {
			continue
		}
		incomingString, incomingIsString := incomingValue.(string)
		existingString, existingIsString := existingValue.(string)
		if incomingIsString && existingIsString && incomingString == "" && existingString != "" {
			incoming[key] = existingString
		}
	}

	merged, err := json.Marshal(incoming)
	if err != nil {
		return "", err
	}
	return string(merged), nil
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}
