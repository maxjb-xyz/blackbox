package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"blackbox/server/internal/correlation"
	"blackbox/server/internal/services"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

type uptimePayload struct {
	Heartbeat struct {
		Status int    `json:"status"`
		Time   string `json:"time"`
		Msg    string `json:"msg"`
	} `json:"heartbeat"`
	Monitor struct {
		Name string `json:"name"`
	} `json:"monitor"`
}

func WebhookUptime(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload uptimePayload
		if !decodeWebhookBody(w, r, 1<<20, &payload) {
			return
		}
		if payload.Monitor.Name == "" {
			writeError(w, http.StatusBadRequest, "monitor.name is required")
			return
		}

		serviceName := services.NormalizeService(database, payload.Monitor.Name)
		ts, timeFallback := parseWebhookTime(payload.Heartbeat.Time)

		meta := map[string]interface{}{
			"monitor": payload.Monitor.Name,
		}
		if timeFallback {
			meta["time_fallback"] = true
		}

		event := "up"
		content := fmt.Sprintf("Monitor '%s' recovered", payload.Monitor.Name)
		correlatedID := ""

		if payload.Heartbeat.Status == 0 {
			event = "down"
			content = fmt.Sprintf("Monitor '%s' is down: %s", payload.Monitor.Name, payload.Heartbeat.Msg)
			meta["status"] = "down"

			if !timeFallback {
				cause, err := correlation.FindCause(database, serviceName, ts)
				if err != nil {
					log.Printf("correlation lookup failed for %s: %v", payload.Monitor.Name, err)
				} else if cause != nil {
					correlatedID = cause.ID
					meta["possible_cause"] = cause.Content
					meta["cause_node"] = cause.NodeName
					meta["cause_event"] = cause.Event
					meta["cause_entry_id"] = cause.ID
				}
			} else {
				log.Printf("skipping correlation for %s due to time fallback", payload.Monitor.Name)
			}
		} else {
			meta["status"] = "up"
			meta["recovery_msg"] = payload.Heartbeat.Msg

			var downEntry types.Entry
			err := database.Where(
				"service = ? AND event = ? AND source = ?",
				serviceName, "down", "webhook",
			).Order("timestamp DESC").First(&downEntry).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
			case err != nil:
				log.Printf("failed to load prior down event for %s: %v", payload.Monitor.Name, err)
			default:
				meta["duration_seconds"] = int64(ts.Sub(downEntry.Timestamp).Seconds())
				meta["down_since"] = downEntry.Timestamp.UTC().Format(time.RFC3339)
			}
		}

		metaBytes, err := json.Marshal(meta)
		if err != nil {
			log.Printf("failed to marshal metadata for monitor %s: %v, meta: %+v", payload.Monitor.Name, err, meta)
			metaBytes = []byte("{}")
		}

		entry := types.Entry{
			ID:           ulid.Make().String(),
			Timestamp:    ts,
			NodeName:     "webhook",
			Source:       "webhook",
			Service:      serviceName,
			Event:        event,
			Content:      content,
			Metadata:     string(metaBytes),
			CorrelatedID: correlatedID,
		}
		if err := database.Create(&entry).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save entry")
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

func parseWebhookTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Now().UTC(), true
	}

	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Now().UTC(), true
	}

	ts = ts.UTC()
	return ts, false
}
