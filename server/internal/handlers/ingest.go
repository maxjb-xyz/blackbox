package handlers

import (
	"encoding/json"
	"net/http"

	"blackbox/shared/types"
	"gorm.io/gorm"
)

func Ingest(database *gorm.DB) http.HandlerFunc {
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
		w.WriteHeader(http.StatusCreated)
	}
}
