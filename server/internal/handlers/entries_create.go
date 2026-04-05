package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/hub"
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

func CreateEntry(database *gorm.DB, h *hub.Hub, incidentCh chan<- types.Entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createEntryRequest
		if !decodeJSONBody(w, r, 1<<20, &req) {
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" {
			writeError(w, http.StatusBadRequest, "title is required")
			return
		}

		timestamp := time.Now().UTC()
		if req.Timestamp != "" {
			parsed, err := time.Parse(time.RFC3339, req.Timestamp)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid timestamp format")
				return
			}
			timestamp = parsed.UTC()
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
		go func(e types.Entry) {
			select {
			case incidentCh <- e:
			case <-time.After(30 * time.Second):
				log.Printf("incidents: channel stalled >30s, skipping incident processing for entry %s (service %s)", e.ID, e.Service)
			}
		}(entry)
		if h != nil {
			if msg := MarshalWSMessage("entry", entry); msg != nil {
				h.Broadcast(msg)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(entry); err != nil {
			log.Printf("failed to encode create entry response for entry %s: %v", entry.ID, err)
		}
	}
}
