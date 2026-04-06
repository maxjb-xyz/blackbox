package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

const ollamaURLKey = "ollama_url"
const ollamaModelKey = "ollama_model"
const ollamaModeKey = "ollama_mode"

func AdminConfig(db *gorm.DB, webhookSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		redactSecrets, err := getFileWatcherRedactSecrets(db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load admin config")
			return
		}
		ollamaURL, ollamaModel, ollamaMode, err := getOllamaSettings(db)
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
			"ollama_mode":                 ollamaMode,
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
			OllamaMode  string `json:"ollama_mode"`
		}
		if !decodeJSONBody(w, r, 1<<20, &req) {
			return
		}

		ollamaURL := strings.TrimSpace(req.OllamaURL)
		if ollamaURL != "" {
			parsed, err := url.ParseRequestURI(ollamaURL)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				writeError(w, http.StatusBadRequest, "ollama_url must be a valid absolute URL")
				return
			}
		}
		ollamaMode := strings.TrimSpace(req.OllamaMode)
		if ollamaMode != "" && ollamaMode != "analysis" && ollamaMode != "enhanced" {
			writeError(w, http.StatusBadRequest, "ollama_mode must be 'analysis' or 'enhanced'")
			return
		}
		if ollamaMode == "" {
			ollamaMode = "analysis"
		}

		now := time.Now()
		settings := []models.AppSetting{
			{Key: ollamaURLKey, Value: ollamaURL, UpdatedAt: now},
			{Key: ollamaModelKey, Value: strings.TrimSpace(req.OllamaModel), UpdatedAt: now},
			{Key: ollamaModeKey, Value: ollamaMode, UpdatedAt: now},
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

func getOllamaSettings(db *gorm.DB) (string, string, string, error) {
	var settings []models.AppSetting
	if err := db.Where("key IN ?", []string{ollamaURLKey, ollamaModelKey, ollamaModeKey}).Find(&settings).Error; err != nil {
		return "", "", "", err
	}

	ollamaURL := ""
	ollamaModel := ""
	ollamaMode := "analysis"
	for _, s := range settings {
		switch s.Key {
		case ollamaURLKey:
			ollamaURL = strings.TrimSpace(s.Value)
		case ollamaModelKey:
			ollamaModel = strings.TrimSpace(s.Value)
		case ollamaModeKey:
			if v := strings.TrimSpace(s.Value); v != "" {
				ollamaMode = v
			}
		}
	}
	return ollamaURL, ollamaModel, ollamaMode, nil
}
