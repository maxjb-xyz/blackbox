package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

type createEntryRequest struct {
	Title     string   `json:"title"`
	Note      string   `json:"note"`
	Services  []string `json:"services"`
	Timestamp string   `json:"timestamp"`
}

func CreateEntry(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createEntryRequest
		if !decodeWebhookBody(w, r, 1<<20, &req) {
			return
		}
		if req.Title == "" {
			writeError(w, http.StatusBadRequest, "title is required")
			return
		}

		timestamp := time.Now().UTC()
		if req.Timestamp != "" {
			if parsed, err := time.Parse(time.RFC3339, req.Timestamp); err == nil {
				timestamp = parsed.UTC()
			}
		}

		services := req.Services
		if services == nil {
			services = []string{}
		}

		service := ""
		if len(services) > 0 {
			service = services[0]
		}

		metaBytes, err := json.Marshal(map[string]interface{}{
			"note":     req.Note,
			"services": services,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encode metadata")
			return
		}

		entry := types.Entry{
			ID:        ulid.Make().String(),
			Timestamp: timestamp,
			NodeName:  "api",
			Source:    "api",
			Service:   service,
			Event:     "manual",
			Content:   req.Title,
			Metadata:  string(metaBytes),
		}
		if err := database.Create(&entry).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save entry")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(entry)
	}
}
