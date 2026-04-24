package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/models"
	"blackbox/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

var validNotifyTypes = map[string]struct{}{
	"discord": {},
	"slack":   {},
	"ntfy":    {},
}

var validNotifyEvents = map[string]struct{}{
	notify.EventIncidentOpenedConfirmed: {},
	notify.EventIncidentOpenedSuspected: {},
	notify.EventIncidentConfirmed:       {},
	notify.EventIncidentResolved:        {},
	notify.EventAIReviewGenerated:       {},
}

func ListNotificationDests(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminRequest(w, r) {
			return
		}

		var dests []models.NotificationDest
		if err := db.Order("created_at ASC").Find(&dests).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list notification destinations")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dests)
	}
}

func CreateNotificationDest(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminRequest(w, r) {
			return
		}

		var req struct {
			Name    string   `json:"name"`
			Type    string   `json:"type"`
			URL     string   `json:"url"`
			Events  []string `json:"events"`
			Enabled bool     `json:"enabled"`
		}
		if !decodeJSONBody(w, r, 16<<10, &req) {
			return
		}

		name, destType, destURL, events, err := validateNotificationDest(req.Name, req.Type, req.URL, req.Events)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		eventsJSON, err := json.Marshal(events)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encode events")
			return
		}

		dest := models.NotificationDest{
			ID:      ulid.Make().String(),
			Name:    name,
			Type:    destType,
			URL:     destURL,
			Events:  string(eventsJSON),
			Enabled: req.Enabled,
		}
		if err := db.Create(&dest).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create notification destination")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(dest)
	}
}

func UpdateNotificationDest(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminRequest(w, r) {
			return
		}

		id := chi.URLParam(r, "id")
		var req struct {
			Name    string   `json:"name"`
			Type    string   `json:"type"`
			URL     string   `json:"url"`
			Events  []string `json:"events"`
			Enabled bool     `json:"enabled"`
		}
		if !decodeJSONBody(w, r, 16<<10, &req) {
			return
		}

		name, destType, destURL, events, err := validateNotificationDest(req.Name, req.Type, req.URL, req.Events)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		eventsJSON, err := json.Marshal(events)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encode events")
			return
		}

		result := db.Model(&models.NotificationDest{}).
			Where("id = ?", id).
			Updates(map[string]interface{}{
				"name":    name,
				"type":    destType,
				"url":     destURL,
				"events":  string(eventsJSON),
				"enabled": req.Enabled,
			})
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to update notification destination")
			return
		}
		if result.RowsAffected == 0 {
			writeError(w, http.StatusNotFound, "notification destination not found")
			return
		}

		var dest models.NotificationDest
		if err := db.First(&dest, "id = ?", id).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load notification destination")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dest)
	}
}

func DeleteNotificationDest(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminRequest(w, r) {
			return
		}

		id := chi.URLParam(r, "id")
		result := db.Delete(&models.NotificationDest{}, "id = ?", id)
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete notification destination")
			return
		}
		if result.RowsAffected == 0 {
			writeError(w, http.StatusNotFound, "notification destination not found")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func TestNotificationDest(db *gorm.DB, dispatcher *notify.Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminRequest(w, r) {
			return
		}
		if dispatcher == nil {
			writeError(w, http.StatusInternalServerError, "notification dispatcher unavailable")
			return
		}

		id := chi.URLParam(r, "id")
		var dest models.NotificationDest
		if err := db.First(&dest, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusNotFound, "notification destination not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to load notification destination")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := dispatcher.SendTest(r.Context(), dest); err != nil {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

func requireAdminRequest(w http.ResponseWriter, r *http.Request) bool {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || !claims.IsAdmin {
		writeError(w, http.StatusForbidden, "admin required")
		return false
	}
	return true
}

func validateNotificationDest(name, destType, destURL string, events []string) (string, string, string, []string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", "", nil, errors.New("name is required")
	}

	destType = strings.TrimSpace(destType)
	if _, ok := validNotifyTypes[destType]; !ok {
		return "", "", "", nil, errors.New("type must be one of: discord, slack, ntfy")
	}

	destURL = strings.TrimSpace(destURL)
	parsed, err := url.Parse(destURL)
	if err != nil || parsed.Host == "" {
		return "", "", "", nil, errors.New("url must be a valid absolute URL")
	}
	if scheme := strings.ToLower(parsed.Scheme); scheme != "http" && scheme != "https" {
		return "", "", "", nil, errors.New("url must be a valid absolute URL")
	}

	cleanEvents := make([]string, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		event = strings.TrimSpace(event)
		if event == "" {
			continue
		}
		if _, ok := validNotifyEvents[event]; !ok {
			return "", "", "", nil, errors.New("unknown event: " + event)
		}
		if _, ok := seen[event]; ok {
			continue
		}
		seen[event] = struct{}{}
		cleanEvents = append(cleanEvents, event)
	}
	if len(cleanEvents) == 0 {
		return "", "", "", nil, errors.New("at least one event must be selected")
	}

	return name, destType, destURL, cleanEvents, nil
}
