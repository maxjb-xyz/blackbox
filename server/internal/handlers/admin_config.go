package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"gorm.io/gorm"
)

func AdminConfig(db *gorm.DB, webhookSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		redactSecrets, err := getFileWatcherRedactSecrets(db)
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
		}); err != nil {
			log.Printf("AdminConfig encode: %v", err)
		}
	}
}
