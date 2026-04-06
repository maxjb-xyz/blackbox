package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
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

func AgentConfig(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeName, ok := middleware.AgentNodeFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		redactSecrets, err := getFileWatcherRedactSecrets(db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load agent config")
			return
		}

		systemdUnits := getSystemdUnitsForNode(db, nodeName)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"file_watcher_redact_secrets": redactSecrets,
			"systemd_units":               systemdUnits,
		})
	}
}

func getSystemdUnitsForNode(db *gorm.DB, nodeName string) []string {
	var config models.SystemdUnitConfig
	if err := db.First(&config, "node_name = ?", nodeName).Error; err != nil {
		return []string{}
	}
	var units []string
	if err := json.Unmarshal([]byte(config.Units), &units); err != nil {
		return []string{}
	}
	return units
}
