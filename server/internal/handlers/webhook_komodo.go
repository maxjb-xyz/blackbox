package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/hub"
	"blackbox/server/internal/middleware"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

type komodoPayload struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

type komodoConfig struct {
	Secret       string            `json:"secret"`
	AllowedTypes []string          `json:"allowed_types"`
	NodeMap      map[string]string `json:"node_map"`
}

func WebhookKomodo(database *gorm.DB, h *hub.Hub, incidentCh chan<- types.Entry, shutdown <-chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			RecordWebhookDelivery(database, "komodo", "", "", "error", "failed to read payload")
			writeError(w, http.StatusBadRequest, "failed to read request body")
			return
		}

		snippet := payloadSnippet(body)

		var payload komodoPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			RecordWebhookDelivery(database, "komodo", snippet, "", "error", "malformed JSON")
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if payload.Type == "" {
			RecordWebhookDelivery(database, "komodo", snippet, "", "ignored", "type is required")
			writeError(w, http.StatusBadRequest, "type is required")
			return
		}

		sourceID, ok := middleware.WebhookSourceIDFromContext(r.Context())
		if !ok {
			RecordWebhookDelivery(database, "komodo", snippet, "", "error", "invalid or missing source")
			writeError(w, http.StatusUnauthorized, "invalid or missing source")
			return
		}
		cfg, found, err := loadKomodoConfig(database, sourceID)
		if err != nil {
			RecordWebhookDelivery(database, "komodo", snippet, "", "error", "failed to load source config")
			writeError(w, http.StatusInternalServerError, "failed to load source config")
			return
		}
		if !found {
			RecordWebhookDelivery(database, "komodo", snippet, "", "error", "invalid or missing source")
			writeError(w, http.StatusUnauthorized, "invalid or missing source")
			return
		}

		if !isAllowedKomodoType(cfg.AllowedTypes, payload.Type) {
			RecordWebhookDelivery(database, "komodo", snippet, "", "ignored", "type not in allowed_types")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		nodeName := resolveKomodoNodeName(payload.Data, cfg.NodeMap)
		serviceName := resolveKomodoService(payload.Data)
		content := buildKomodoContent(payload.Type, serviceName)
		meta := buildKomodoMetadata(payload.Type, payload.Data)

		metaBytes, err := json.Marshal(meta)
		if err != nil {
			log.Printf("WebhookKomodo: failed to marshal metadata: %v", err)
			metaBytes = []byte("{}")
		}

		entry := types.Entry{
			ID:        ulid.Make().String(),
			Timestamp: time.Now().UTC(),
			NodeName:  nodeName,
			Source:    "komodo",
			Service:   serviceName,
			Event:     strings.ToLower(payload.Type),
			Content:   content,
			Metadata:  string(metaBytes),
		}

		if err := database.Create(&entry).Error; err != nil {
			RecordWebhookDelivery(database, "komodo", snippet, "", "error", "failed to save entry")
			writeError(w, http.StatusInternalServerError, "failed to save entry")
			return
		}

		dispatchToIncidentChannelWithShutdown(incidentCh, shutdown, entry)
		if h != nil {
			if msg := MarshalWSMessage("entry", entry); msg != nil {
				h.Broadcast(msg)
			}
		}

		RecordWebhookDelivery(database, "komodo", snippet, "", "processed", "")
		w.WriteHeader(http.StatusCreated)
	}
}

func loadKomodoConfig(db *gorm.DB, sourceID string) (komodoConfig, bool, error) {
	if sourceID == "" {
		return komodoConfig{}, false, nil
	}
	var inst models.DataSourceInstance
	if err := db.First(&inst, "id = ? AND type = ? AND enabled = ?", sourceID, "webhook_komodo", true).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return komodoConfig{}, false, nil
		}
		return komodoConfig{}, false, err
	}
	var cfg komodoConfig
	if err := json.Unmarshal([]byte(inst.Config), &cfg); err != nil {
		return komodoConfig{}, false, err
	}
	return cfg, true, nil
}

func isAllowedKomodoType(allowed []string, eventType string) bool {
	norm := strings.ToLower(strings.TrimSpace(eventType))
	for _, t := range allowed {
		if strings.ToLower(strings.TrimSpace(t)) == norm {
			return true
		}
	}
	return false
}

func resolveKomodoNodeName(data map[string]any, nodeMap map[string]string) string {
	raw := ""
	if v, ok := data["server_name"]; ok {
		if s, ok := v.(string); ok {
			raw = strings.TrimSpace(s)
		}
	}
	if raw == "" {
		if v, ok := data["name"]; ok {
			if s, ok := v.(string); ok {
				raw = strings.TrimSpace(s)
			}
		}
	}
	if raw == "" {
		return "komodo"
	}
	if mapped, ok := nodeMap[strings.ToLower(raw)]; ok {
		return mapped
	}
	return raw
}

func resolveKomodoService(data map[string]any) string {
	if v, ok := data["name"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return "komodo"
}

func buildKomodoContent(eventType, serviceName string) string {
	label := camelToWords(eventType)
	if serviceName != "komodo" {
		return label + ": " + serviceName
	}
	return label
}

func camelToWords(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func buildKomodoMetadata(eventType string, data map[string]any) map[string]any {
	meta := map[string]any{
		"komodo.type": eventType,
	}
	knownFields := map[string]string{
		"name":        "komodo.name",
		"server_name": "komodo.server_name",
		"id":          "komodo.id",
	}
	extra := map[string]any{}
	for k, v := range data {
		if metaKey, ok := knownFields[k]; ok {
			meta[metaKey] = v
		} else {
			extra[k] = v
		}
	}
	if len(extra) > 0 {
		meta["komodo.raw"] = extra
	}
	return meta
}
