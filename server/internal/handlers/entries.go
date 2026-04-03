package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"blackbox/shared/types"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func ListEntries(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		limitStr := r.URL.Query().Get("limit")
		node := r.URL.Query().Get("node")
		source := r.URL.Query().Get("source")
		q := r.URL.Query().Get("q")

		limit := 50
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}

		tx := database.Model(&types.Entry{}).Order("id DESC").Limit(limit + 1)
		if cursor != "" {
			tx = tx.Where("id < ?", cursor)
		}
		if node != "" {
			tx = tx.Where("node_name = ?", node)
		}
		if source != "" {
			tx = tx.Where("source = ?", source)
		}
		if q != "" {
			like := "%" + q + "%"
			tx = tx.Where("content LIKE ? OR service LIKE ?", like, like)
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
			resp.NextCursor = entries[limit].ID
			resp.Entries = entries[:limit]
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
