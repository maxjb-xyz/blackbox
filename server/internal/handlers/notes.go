package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

const (
	maxNoteLength    = 2000
	defaultNotesPage = 50
	maxNotesPage     = 100
)

func CreateNote(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		entryID := chi.URLParam(r, "id")
		var entry types.Entry
		if err := database.First(&entry, "id = ?", entryID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusNotFound, "entry not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to fetch entry")
			return
		}

		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		content := strings.TrimSpace(body.Content)
		if content == "" {
			writeError(w, http.StatusBadRequest, "content is required")
			return
		}
		if utf8.RuneCountInString(content) > maxNoteLength {
			writeError(w, http.StatusBadRequest, "content exceeds max length")
			return
		}

		note := models.EntryNote{
			ID:        ulid.Make().String(),
			EntryID:   entryID,
			UserID:    claims.UserID,
			Username:  claims.Username,
			Content:   content,
			CreatedAt: time.Now().UTC(),
		}
		if err := database.Create(&note).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save note")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(note); err != nil {
			log.Printf("CreateNote encode: %v", err)
		}
	}
}

func ListNotes(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entryID := chi.URLParam(r, "id")
		limit := defaultNotesPage
		if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
			if parsedLimit, err := strconv.Atoi(rawLimit); err == nil && parsedLimit > 0 {
				if parsedLimit > maxNotesPage {
					limit = maxNotesPage
				} else {
					limit = parsedLimit
				}
			}
		}

		offset := 0
		if rawOffset := r.URL.Query().Get("offset"); rawOffset != "" {
			if parsedOffset, err := strconv.Atoi(rawOffset); err == nil && parsedOffset >= 0 {
				offset = parsedOffset
			}
		}

		var notes []models.EntryNote
		if err := database.
			Where("entry_id = ?", entryID).
			Order("created_at ASC").
			Order("id ASC").
			Offset(offset).
			Limit(limit + 1).
			Find(&notes).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch notes")
			return
		}
		if notes == nil {
			notes = []models.EntryNote{}
		}

		hasMore := len(notes) > limit
		if hasMore {
			notes = notes[:limit]
		}

		response := struct {
			Notes      []models.EntryNote `json:"notes"`
			HasMore    bool               `json:"has_more"`
			NextOffset int                `json:"next_offset,omitempty"`
		}{
			Notes:   notes,
			HasMore: hasMore,
		}
		if hasMore {
			response.NextOffset = offset + len(notes)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("ListNotes encode: %v", err)
		}
	}
}

func DeleteNote(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		noteID := chi.URLParam(r, "id")

		result := database.Where("id = ? AND user_id = ?", noteID, claims.UserID).Delete(&models.EntryNote{})
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete note")
			return
		}
		if result.RowsAffected == 0 {
			var note models.EntryNote
			if err := database.First(&note, "id = ?", noteID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					writeError(w, http.StatusNotFound, "note not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to fetch note")
				return
			}
			writeError(w, http.StatusForbidden, "not your note")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
