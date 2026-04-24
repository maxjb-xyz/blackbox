package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"blackbox/server/internal/hub"
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
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			RecordWebhookDelivery(database, "watchtower", "", "", "error", "failed to read payload")
			writeError(w, http.StatusBadRequest, "failed to read request body")
			return
		}

		var payload watchtowerPayload
		snippet := payloadSnippet(body)
		if err := json.Unmarshal(body, &payload); err != nil {
			RecordWebhookDelivery(database, "watchtower", snippet, "", "error", "malformed JSON")
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if payload.Message == "" {
			RecordWebhookDelivery(database, "watchtower", snippet, "", "ignored", "Message is required")
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
		updatedServices := extractWatchtowerServices(payload.Message)
		if len(updatedServices) > 0 {
			meta["watchtower.services"] = updatedServices
		}

		metaBytes, err := json.Marshal(meta)
		if err != nil {
			log.Printf("failed to marshal metadata for watchtower: %v, meta: %+v", err, meta)
			metaBytes = []byte("{}")
		}

		serviceName := "watchtower"

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
			RecordWebhookDelivery(database, "watchtower", snippet, "", "error", "failed to save entry")
			writeError(w, http.StatusInternalServerError, "failed to save entry")
			return
		}
		dispatchToIncidentChannelWithShutdown(incidentCh, shutdown, entry)
		if h != nil {
			if msg := MarshalWSMessage("entry", entry); msg != nil {
				h.Broadcast(msg)
			}
		}

		RecordWebhookDelivery(database, "watchtower", snippet, "", "processed", "")
		w.WriteHeader(http.StatusCreated)
	}
}

func extractWatchtowerServices(message string) []string {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}

	candidateSection := message
	idx := strings.Index(candidateSection, ":")
	if idx < 0 {
		return nil
	}
	candidateSection = candidateSection[idx+1:]
	candidateSection = strings.TrimSpace(candidateSection)
	if candidateSection == "" {
		return nil
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

		normalized := strings.ToLower(strings.TrimSpace(part))
		if normalized == "" {
			continue
		}
		if _, ok := servicesSeen[normalized]; ok {
			continue
		}
		servicesSeen[normalized] = struct{}{}
		servicesList = append(servicesList, normalized)
	}

	return servicesList
}
