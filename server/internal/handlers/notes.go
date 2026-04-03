package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
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

		var notes []models.EntryNote
		if err := database.Where("entry_id = ?", entryID).Order("created_at ASC").Find(&notes).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch notes")
			return
		}
		if notes == nil {
			notes = []models.EntryNote{}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(notes); err != nil {
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

		var note models.EntryNote
		if err := database.First(&note, "id = ?", noteID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusNotFound, "note not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to fetch note")
			return
		}

		if note.UserID != claims.UserID {
			writeError(w, http.StatusForbidden, "not your note")
			return
		}

		if err := database.Delete(&note).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete note")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
