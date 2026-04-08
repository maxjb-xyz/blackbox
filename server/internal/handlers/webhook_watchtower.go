package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"blackbox/server/internal/hub"
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

var watchtowerParensPattern = regexp.MustCompile(`\([^)]*\)`)

func WebhookWatchtower(database *gorm.DB, h *hub.Hub, incidentCh chan<- types.Entry, shutdown <-chan struct{}) http.HandlerFunc {
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
		updatedServices, err := extractWatchtowerServices(database, payload.Message)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to normalize watchtower services")
			return
		}
		if len(updatedServices) > 0 {
			meta["watchtower.services"] = updatedServices
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
		dispatchToIncidentChannelWithShutdown(incidentCh, shutdown, entry)
		if h != nil {
			if msg := MarshalWSMessage("entry", entry); msg != nil {
				h.Broadcast(msg)
			}
		}

		w.WriteHeader(http.StatusCreated)
	}
}

func extractWatchtowerServices(database *gorm.DB, message string) ([]string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, nil
	}

	candidateSection := message
	idx := strings.Index(candidateSection, ":")
	if idx < 0 {
		return nil, nil
	}
	candidateSection = candidateSection[idx+1:]
	candidateSection = strings.TrimSpace(candidateSection)
	if candidateSection == "" {
		return nil, nil
	}

	parts := strings.FieldsFunc(candidateSection, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	servicesSeen := make(map[string]struct{}, len(parts))
	servicesList := make([]string, 0, len(parts))

	for _, part := range parts {
		part = watchtowerParensPattern.ReplaceAllString(part, "")
		part = strings.TrimSpace(strings.Trim(part, `"'[]`))
		if part == "" {
			continue
		}

		normalized, err := services.NormalizeService(database, part)
		if err != nil {
			return nil, err
		}
		if normalized == "" {
			continue
		}
		if _, ok := servicesSeen[normalized]; ok {
			continue
		}
		servicesSeen[normalized] = struct{}{}
		servicesList = append(servicesList, normalized)
	}

	return servicesList, nil
}
