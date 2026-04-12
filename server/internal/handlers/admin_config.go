package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

const aiProviderKey = "ai_provider"
const aiURLKey = "ai_url"
const aiModelKey = "ai_model"
const aiAPIKeyKey = "ai_api_key"
const aiModeKey = "ai_mode"

// Legacy keys — read-only fallback for existing installs
const legacyOllamaURLKey = "ollama_url"
const legacyOllamaModelKey = "ollama_model"
const legacyOllamaModeKey = "ollama_mode"

func AdminConfig(db *gorm.DB, webhookSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		redactSecrets, err := getFileWatcherRedactSecrets(db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load admin config")
			return
		}
		ai, err := getAISettings(db)
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
			"ai_provider":                 ai.provider,
			"ai_url":                      ai.url,
			"ai_model":                    ai.model,
			"ai_api_key_set":              ai.apiKeySet,
			"ai_mode":                     ai.mode,
		}); err != nil {
			log.Printf("AdminConfig encode: %v", err)
		}
	}
}

func UpdateAISettings(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Provider    string `json:"ai_provider"`
			URL         string `json:"ai_url"`
			Model       string `json:"ai_model"`
			APIKey      string `json:"ai_api_key"`
			ClearAPIKey bool   `json:"ai_clear_api_key"`
			Mode        string `json:"ai_mode"`
		}
		if !decodeJSONBody(w, r, 1<<20, &req) {
			return
		}

		provider := strings.TrimSpace(req.Provider)
		if provider == "" {
			provider = "ollama"
		}
		if provider != "ollama" && provider != "openai_compat" {
			writeError(w, http.StatusBadRequest, "ai_provider must be 'ollama' or 'openai_compat'")
			return
		}

		aiURL := strings.TrimSpace(req.URL)
		if aiURL != "" {
			parsed, err := url.ParseRequestURI(aiURL)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				writeError(w, http.StatusBadRequest, "ai_url must be a valid absolute URL")
				return
			}
		}

		mode := strings.TrimSpace(req.Mode)
		if mode != "" && mode != "analysis" && mode != "enhanced" {
			writeError(w, http.StatusBadRequest, "ai_mode must be 'analysis' or 'enhanced'")
			return
		}
		if mode == "" {
			mode = "analysis"
		}

		// Blank key: preserve existing unless ClearAPIKey is explicitly true.
		apiKey := strings.TrimSpace(req.APIKey)
		if apiKey == "" && !req.ClearAPIKey {
			var existing models.AppSetting
			if err := db.First(&existing, "key = ?", aiAPIKeyKey).Error; err == nil {
				apiKey = existing.Value
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusInternalServerError, "failed to load existing api key")
				return
			}
		}

		now := time.Now()
		settings := []models.AppSetting{
			{Key: aiProviderKey, Value: provider, UpdatedAt: now},
			{Key: aiURLKey, Value: aiURL, UpdatedAt: now},
			{Key: aiModelKey, Value: strings.TrimSpace(req.Model), UpdatedAt: now},
			{Key: aiAPIKeyKey, Value: apiKey, UpdatedAt: now},
			{Key: aiModeKey, Value: mode, UpdatedAt: now},
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			for _, s := range settings {
				if err := tx.Save(&s).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save setting")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// UpdateOllamaSettingsLegacy handles PUT /api/admin/settings/ollama (deprecated).
// Translates legacy ollama_* field names to ai_* and saves using the new schema,
// always preserving any stored API key since the old endpoint had no key concept.
func UpdateOllamaSettingsLegacy(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OllamaURL   string `json:"ollama_url"`
			OllamaModel string `json:"ollama_model"`
			OllamaMode  string `json:"ollama_mode"`
		}
		if !decodeJSONBody(w, r, 1<<20, &req) {
			return
		}

		aiURL := strings.TrimSpace(req.OllamaURL)
		if aiURL != "" {
			parsed, err := url.ParseRequestURI(aiURL)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				writeError(w, http.StatusBadRequest, "ollama_url must be a valid absolute URL")
				return
			}
		}

		mode := strings.TrimSpace(req.OllamaMode)
		if mode != "" && mode != "analysis" && mode != "enhanced" {
			writeError(w, http.StatusBadRequest, "ollama_mode must be 'analysis' or 'enhanced'")
			return
		}
		if mode == "" {
			mode = "analysis"
		}

		// Preserve any existing API key — legacy endpoint has no key field.
		var apiKey string
		var existing models.AppSetting
		if err := db.First(&existing, "key = ?", aiAPIKeyKey).Error; err == nil {
			apiKey = existing.Value
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(w, http.StatusInternalServerError, "failed to load existing api key")
			return
		}

		now := time.Now()
		settings := []models.AppSetting{
			{Key: aiProviderKey, Value: "ollama", UpdatedAt: now},
			{Key: aiURLKey, Value: aiURL, UpdatedAt: now},
			{Key: aiModelKey, Value: strings.TrimSpace(req.OllamaModel), UpdatedAt: now},
			{Key: aiAPIKeyKey, Value: apiKey, UpdatedAt: now},
			{Key: aiModeKey, Value: mode, UpdatedAt: now},
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			for _, s := range settings {
				if err := tx.Save(&s).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save setting")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

type aiSettingsResult struct {
	provider  string
	url       string
	model     string
	apiKeySet bool
	mode      string
}

func getAISettings(db *gorm.DB) (aiSettingsResult, error) {
	keys := []string{aiProviderKey, aiURLKey, aiModelKey, aiAPIKeyKey, aiModeKey, legacyOllamaURLKey, legacyOllamaModelKey, legacyOllamaModeKey}
	var settings []models.AppSetting
	if err := db.Where("key IN ?", keys).Find(&settings).Error; err != nil {
		return aiSettingsResult{}, err
	}

	m := make(map[string]string, len(settings))
	for _, s := range settings {
		m[s.Key] = strings.TrimSpace(s.Value)
	}

	result := aiSettingsResult{mode: "analysis"}

	result.provider = m[aiProviderKey]
	if result.provider == "" {
		result.provider = "ollama"
	}

	if url, ok := m[aiURLKey]; ok {
		result.url = url
	} else {
		result.url = m[legacyOllamaURLKey]
	}

	if model, ok := m[aiModelKey]; ok {
		result.model = model
	} else {
		result.model = m[legacyOllamaModelKey]
	}

	result.apiKeySet = m[aiAPIKeyKey] != ""

	if _, ok := m[aiModeKey]; ok {
		if v := m[aiModeKey]; v != "" {
			result.mode = v
		}
	} else if v := m[legacyOllamaModeKey]; v != "" {
		result.mode = v
	}

	return result, nil
}
