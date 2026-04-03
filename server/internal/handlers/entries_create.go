package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	servicealiases "blackbox/server/internal/services"
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
		req.Title = strings.TrimSpace(req.Title)
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

		serviceNames := req.Services
		if serviceNames == nil {
			serviceNames = []string{}
		}

		normalizedServices := make([]string, 0, len(serviceNames))
		for _, name := range serviceNames {
			normalized, err := servicealiases.NormalizeService(database, name)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to normalize service")
				return
			}
			if normalized != "" {
				normalizedServices = append(normalizedServices, normalized)
			}
		}

		service := ""
		if len(normalizedServices) > 0 {
			service = normalizedServices[0]
		}

		metaBytes, err := json.Marshal(map[string]interface{}{
			"note":     req.Note,
			"services": normalizedServices,
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
