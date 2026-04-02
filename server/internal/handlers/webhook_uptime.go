package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"blackbox/server/internal/correlation"
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
		maxBytes := int64(1 << 20) // 1MB limit
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

		var payload uptimePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			if err.Error() == "http: request body too large" {
				writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if payload.Monitor.Name == "" {
			writeError(w, http.StatusBadRequest, "monitor.name is required")
			return
		}

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
				cause, err := correlation.FindCause(database, payload.Monitor.Name, ts)
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
		}

		metaBytes, _ := json.Marshal(meta)

		entry := types.Entry{
			ID:           ulid.Make().String(),
			Timestamp:    ts,
			NodeName:     "webhook",
			Source:       "webhook",
			Service:      payload.Monitor.Name,
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

	return ts, false
}