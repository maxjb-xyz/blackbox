package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

const ollamaURLKey = "ollama_url"
const ollamaModelKey = "ollama_model"

func AdminConfig(db *gorm.DB, webhookSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		redactSecrets, err := getFileWatcherRedactSecrets(db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load admin config")
			return
		}
		ollamaURL, ollamaModel, err := getOllamaSettings(db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load admin config")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"webhook_secret":              webhookSecret,
			"file_watcher_redact_secrets": redactSecrets,
			"ollama_url":                  ollamaURL,
			"ollama_model":                ollamaModel,
		}); err != nil {
			log.Printf("AdminConfig encode: %v", err)
		}
	}
}

func UpdateOllamaSettings(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OllamaURL   string `json:"ollama_url"`
			OllamaModel string `json:"ollama_model"`
		}
		if !decodeJSONBody(w, r, 1<<20, &req) {
			return
		}

		now := time.Now()
		settings := []models.AppSetting{
			{Key: ollamaURLKey, Value: strings.TrimSpace(req.OllamaURL), UpdatedAt: now},
			{Key: ollamaModelKey, Value: strings.TrimSpace(req.OllamaModel), UpdatedAt: now},
		}
		for _, s := range settings {
			if err := db.Save(&s).Error; err != nil {
				writeError(w, http.StatusInternalServerError, "failed to save setting")
				return
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func getOllamaSettings(db *gorm.DB) (string, string, error) {
	var settings []models.AppSetting
	if err := db.Where("key IN ?", []string{ollamaURLKey, ollamaModelKey}).Find(&settings).Error; err != nil {
		return "", "", err
	}

	ollamaURL := ""
	ollamaModel := ""
	for _, s := range settings {
		switch s.Key {
		case ollamaURLKey:
			ollamaURL = strings.TrimSpace(s.Value)
		case ollamaModelKey:
			ollamaModel = strings.TrimSpace(s.Value)
		}
	}
	return ollamaURL, ollamaModel, nil
}
