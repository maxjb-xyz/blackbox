package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"blackbox/server/internal/services"
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
		if !decodeJSONBody(w, r, 1<<20, &payload) {
			return
		}

		if payload.Message == "" {
			writeError(w, http.StatusBadRequest, "Message is required")
			return
		}

		meta := make(map[string]interface{})
		if payload.Title != "" {
			meta["watchtower.title"] = payload.Title
		}
		if payload.Level != "" {
			meta["watchtower.level"] = payload.Level
		}

		metaBytes, err := json.Marshal(meta)
		if err != nil {
			log.Printf("failed to marshal metadata for watchtower: %v, meta: %+v", err, meta)
			metaBytes = []byte("{}")
		}

		serviceName, err := services.NormalizeService(database, "watchtower")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to normalize service")
			return
		}

		entry := types.Entry{
			ID:        ulid.Make().String(),
			Timestamp: time.Now().UTC(),
			NodeName:  "webhook",
			Source:    "webhook",
			Service:   serviceName,
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
