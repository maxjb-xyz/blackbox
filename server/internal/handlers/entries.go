package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"blackbox/shared/types"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type entryCursor struct {
	Timestamp time.Time
	ID        string
}

func ListEntries(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		limitStr := r.URL.Query().Get("limit")
		node := r.URL.Query().Get("node")
		source := r.URL.Query().Get("source")
		service := r.URL.Query().Get("service")
		q := r.URL.Query().Get("q")
		timeStartStr := r.URL.Query().Get("time_start")
		timeEndStr := r.URL.Query().Get("time_end")

		limit := 50
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}

		tx := database.Model(&types.Entry{}).Order("timestamp DESC").Order("id DESC").Limit(limit + 1)
		if cursor != "" {
			parsedCursor, err := parseEntryCursor(cursor)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid cursor")
				return
			}
			tx = tx.Where(
				"timestamp < ? OR (timestamp = ? AND id < ?)",
				parsedCursor.Timestamp,
				parsedCursor.Timestamp,
				parsedCursor.ID,
			)
		}
		if timeStartStr != "" {
			parsedTimeStart, err := time.Parse(time.RFC3339Nano, timeStartStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid time_start")
				return
			}
			tx = tx.Where("timestamp >= ?", parsedTimeStart)
		}
		if timeEndStr != "" {
			parsedTimeEnd, err := time.Parse(time.RFC3339Nano, timeEndStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid time_end")
				return
			}
			tx = tx.Where("timestamp <= ?", parsedTimeEnd)
		}
		if node != "" {
			tx = tx.Where("node_name = ?", node)
		}
		if source != "" {
			tx = tx.Where("source = ?", source)
		}
		if service != "" {
			tx = tx.Where("service = ?", service)
		}
		if q != "" {
			like := "%" + q + "%"
			tx = tx.Where("content LIKE ? OR service LIKE ?", like, like)
		}
		hideHeartbeat := r.URL.Query().Get("hide_heartbeat") == "true"
		if hideHeartbeat {
			tx = tx.Where("NOT (source = 'agent' AND event = 'heartbeat')")
		}

		var entries []types.Entry
		if err := tx.Find(&entries).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch entries")
			return
		}

		type response struct {
			Entries    []types.Entry `json:"entries"`
			NextCursor string        `json:"next_cursor,omitempty"`
		}

		resp := response{Entries: entries}
		if len(entries) > limit {
			resp.Entries = entries[:limit]
			resp.NextCursor = encodeEntryCursor(resp.Entries[len(resp.Entries)-1])
		}
		if resp.Entries == nil {
			resp.Entries = []types.Entry{}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("ListEntries encode: %v", err)
		}
	}
}

func ListEntryServices(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var services []string
		if err := database.Model(&types.Entry{}).
			Distinct().
			Where("service != ''").
			Order("service ASC").
			Pluck("service", &services).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list services")
			return
		}
		if services == nil {
			services = []string{}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string][]string{"services": services}); err != nil {
			log.Printf("ListEntryServices encode: %v", err)
		}
	}
}

func encodeEntryCursor(entry types.Entry) string {
	return entry.Timestamp.UTC().Format(time.RFC3339Nano) + "|" + entry.ID
}

func parseEntryCursor(cursor string) (entryCursor, error) {
	ts, id, ok := strings.Cut(cursor, "|")
	if !ok || id == "" {
		return entryCursor{}, errors.New("cursor missing delimiter")
	}
	parsedTime, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return entryCursor{}, err
	}
	return entryCursor{
		Timestamp: parsedTime.UTC(),
		ID:        id,
	}, nil
}

func GetEntry(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var entry types.Entry
		if err := database.First(&entry, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusNotFound, "entry not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to fetch entry")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(entry); err != nil {
			log.Printf("GetEntry encode: %v", err)
		}
	}
}
