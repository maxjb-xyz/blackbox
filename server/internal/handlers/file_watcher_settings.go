package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/middleware"
	"blackbox/server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const fileWatcherRedactSecretsKey = "file_watcher_redact_secrets"

func getFileWatcherRedactSecrets(db *gorm.DB) (bool, error) {
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", fileWatcherRedactSecretsKey).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		return false, err
	}
	switch setting.Value {
	case "", "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, errors.New("invalid file watcher redaction value")
	}
}

func setFileWatcherRedactSecrets(db *gorm.DB, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	setting := models.AppSetting{
		Key:       fileWatcherRedactSecretsKey,
		Value:     value,
		UpdatedAt: time.Now(),
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"value":      value,
			"updated_at": time.Now(),
		}),
	}).Create(&setting).Error
}

func UpdateFileWatcherSettings(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok || !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}

		var req struct {
			RedactSecrets *bool `json:"redact_secrets"`
		}
		if !decodeJSONBody(w, r, 8<<10, &req) {
			return
		}
		if req.RedactSecrets == nil {
			writeError(w, http.StatusBadRequest, "redact_secrets is required")
			return
		}

		if err := setFileWatcherRedactSecrets(db, *req.RedactSecrets); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update file watcher settings")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"redact_secrets": *req.RedactSecrets})
	}
}

// getFileWatcherRedactSecretsForNode reads from data_source_instances first (type="filewatcher",
// enabled=true, node_id=nodeName). Disabled per-node sources are intentionally ignored, so callers
// fall back to the global AppSetting whenever the node-specific source is absent or disabled.
func getFileWatcherRedactSecretsForNode(db *gorm.DB, nodeName string) (bool, error) {
	var inst models.DataSourceInstance
	err := db.Where("type = ? AND enabled = ? AND node_id = ?", "filewatcher", true, nodeName).First(&inst).Error
	if err == nil {
		var cfg struct {
			RedactSecrets *bool `json:"redact_secrets"`
		}
		if jsonErr := json.Unmarshal([]byte(inst.Config), &cfg); jsonErr == nil {
			if cfg.RedactSecrets != nil {
				return *cfg.RedactSecrets, nil
			}
		} else {
			log.Printf("getFileWatcherRedactSecretsForNode: failed to parse config for source %s: %v", inst.ID, jsonErr)
		}
	}
	// Fall back to global setting
	return getFileWatcherRedactSecrets(db)
}

// getSystemdUnitsForNode reads from data_source_instances first (type="systemd",
// enabled=true, node_id=nodeName). Disabled per-node sources are intentionally ignored, so callers
// fall back to the legacy systemd_unit_configs table whenever the node-specific source is absent or disabled.
func getSystemdUnitsForNode(db *gorm.DB, nodeName string) ([]string, error) {
	var inst models.DataSourceInstance
	err := db.Where("type = ? AND enabled = ? AND node_id = ?", "systemd", true, nodeName).First(&inst).Error
	if err == nil {
		var cfg struct {
			Units []string `json:"units"`
		}
		if jsonErr := json.Unmarshal([]byte(inst.Config), &cfg); jsonErr == nil {
			if cfg.Units == nil {
				cfg.Units = []string{}
			}
			return cfg.Units, nil
		} else {
			log.Printf("getSystemdUnitsForNode: failed to parse config for source %s: %v", inst.ID, jsonErr)
		}
	}
	// Fall back to legacy table
	var config models.SystemdUnitConfig
	if err := db.First(&config, "node_name = ?", nodeName).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []string{}, nil
		}
		return nil, err
	}
	var units []string
	if err := json.Unmarshal([]byte(config.Units), &units); err != nil {
		return nil, err
	}
	return units, nil
}

func AgentConfig(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// AgentAuth must set the node name in context; unauthenticated requests are rejected.
		nodeName, ok := middleware.AgentNodeFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		if capsHeader := r.Header.Get("X-Blackbox-Agent-Capabilities"); capsHeader != "" {
			const maxCapsHeader = 4 * 1024 // 4 KiB
			if len(capsHeader) > maxCapsHeader {
				capsHeader = capsHeader[:maxCapsHeader]
				if idx := strings.LastIndex(capsHeader, ","); idx >= 0 {
					capsHeader = capsHeader[:idx]
				} else {
					capsHeader = ""
				}
			}
			parts := strings.Split(capsHeader, ",")
			const maxCaps = 32
			const maxCapLen = 64
			caps := make([]string, 0, min(len(parts), maxCaps))
			for _, c := range parts {
				c = strings.TrimSpace(c)
				if c == "" {
					continue
				}
				if len(c) > maxCapLen {
					c = c[:maxCapLen]
				}
				caps = append(caps, c)
				if len(caps) >= maxCaps {
					break
				}
			}
			if capsJSON, err := json.Marshal(caps); err == nil {
				newValue := string(capsJSON)
				result := db.Model(&models.Node{}).
					Where("name = ? AND (capabilities IS NULL OR capabilities != ?)", nodeName, newValue).
					Update("capabilities", newValue)
				if result.Error != nil {
					log.Printf("AgentConfig: failed to store capabilities for node %s: %v", nodeName, result.Error)
				}
			}
		}

		redactSecrets, err := getFileWatcherRedactSecretsForNode(db, nodeName)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load agent config")
			return
		}

		systemdUnits, err := getSystemdUnitsForNode(db, nodeName)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load agent config")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"file_watcher_redact_secrets": redactSecrets,
			"systemd_units":               systemdUnits,
		})
	}
}
