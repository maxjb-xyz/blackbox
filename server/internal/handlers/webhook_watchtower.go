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
		maxBytes := int64(1 << 20) // 1MB limit
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

		var payload watchtowerPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			if err.Error() == "http: request body too large" {
				writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		meta := make(map[string]interface{})
		if payload.Title != "" {
			meta["watchtower.title"] = payload.Title
		}
		if payload.Level != "" {
			meta["watchtower.level"] = payload.Level
		}

		metaBytes, _ := json.Marshal(meta)

		entry := types.Entry{
			ID:        ulid.Make().String(),
			Timestamp: time.Now().UTC(),
			NodeName:  "webhook",
			Source:    "webhook",
			Service:   "watchtower",
			Event:     "update",
			Content:   payload.Message,
			Metadata:  string(metaBytes),
		}
		if err := database.Create(&entry).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save entry")
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}