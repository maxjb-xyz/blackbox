package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

type watchtowerPayload struct {
	Title   string `json:"Title"`
	Message string `json:"Message"`
	Level   string `json:"Level"`
}

func WebhookWatchtower(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload watchtowerPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		entry := types.Entry{
			ID:        ulid.Make().String(),
			Timestamp: time.Now().UTC(),
			NodeName:  "webhook",
			Source:    "webhook",
			Service:   "watchtower",
			Event:     "update",
			Content:   payload.Message,
		}
		if err := database.Create(&entry).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save entry")
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}
