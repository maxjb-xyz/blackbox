package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"blackbox/server/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

const (
	playbookMaxNameLen       = 100
	playbookMaxPatternLen    = 200
	playbookMaxContentBytes  = 128 << 10 // 128 KiB of markdown is plenty for a runbook
	playbookRequestBodyLimit = playbookMaxContentBytes + 4<<10
	playbookMinPriority      = -1000
	playbookMaxPriority      = 1000
)

type playbookRequest struct {
	Name           string `json:"name"`
	ServicePattern string `json:"service_pattern"`
	Priority       int    `json:"priority"`
	ContentMD      string `json:"content_md"`
	Enabled        bool   `json:"enabled"`
}

func ListPlaybooks(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminRequest(w, r) {
			return
		}
		var playbooks []models.Playbook
		if err := database.Order("priority DESC, name ASC").Find(&playbooks).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list playbooks")
			return
		}
		if playbooks == nil {
			playbooks = []models.Playbook{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(playbooks)
	}
}

func CreatePlaybook(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminRequest(w, r) {
			return
		}
		var req playbookRequest
		if !decodeJSONBody(w, r, playbookRequestBodyLimit, &req) {
			return
		}
		if err := validatePlaybookRequest(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		now := time.Now().UTC()
		playbook := models.Playbook{
			ID:             ulid.Make().String(),
			Name:           req.Name,
			ServicePattern: req.ServicePattern,
			Priority:       req.Priority,
			ContentMD:      req.ContentMD,
			Enabled:        req.Enabled,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := database.Create(&playbook).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create playbook")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(playbook)
	}
}

func UpdatePlaybook(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminRequest(w, r) {
			return
		}
		id := chi.URLParam(r, "id")
		var req playbookRequest
		if !decodeJSONBody(w, r, playbookRequestBodyLimit, &req) {
			return
		}
		if err := validatePlaybookRequest(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		updates := map[string]any{
			"name":            req.Name,
			"service_pattern": req.ServicePattern,
			"priority":        req.Priority,
			"content_md":      req.ContentMD,
			"enabled":         req.Enabled,
			"updated_at":      time.Now().UTC(),
		}
		result := database.Model(&models.Playbook{}).Where("id = ?", id).Updates(updates)
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to update playbook")
			return
		}
		if result.RowsAffected == 0 {
			writeError(w, http.StatusNotFound, "playbook not found")
			return
		}

		var playbook models.Playbook
		if err := database.First(&playbook, "id = ?", id).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to reload playbook")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(playbook)
	}
}

func DeletePlaybook(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminRequest(w, r) {
			return
		}
		id := chi.URLParam(r, "id")
		result := database.Delete(&models.Playbook{}, "id = ?", id)
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete playbook")
			return
		}
		if result.RowsAffected == 0 {
			writeError(w, http.StatusNotFound, "playbook not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func validatePlaybookRequest(req *playbookRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.ServicePattern = strings.TrimSpace(req.ServicePattern)

	if req.Name == "" {
		return errors.New("name is required")
	}
	if len(req.Name) > playbookMaxNameLen {
		return errors.New("name exceeds 100 characters")
	}
	if req.ServicePattern == "" {
		return errors.New("service_pattern is required")
	}
	if len(req.ServicePattern) > playbookMaxPatternLen {
		return errors.New("service_pattern exceeds 200 characters")
	}
	// Reject patterns path.Match can't parse — fail fast instead of silently
	// skipping the playbook at match time.
	if _, err := path.Match(req.ServicePattern, ""); err != nil {
		return fmt.Errorf("service_pattern is not a valid glob: %w", err)
	}
	if req.ContentMD == "" {
		return errors.New("content_md is required")
	}
	if len(req.ContentMD) > playbookMaxContentBytes {
		return errors.New("content_md exceeds 128 KiB")
	}
	if req.Priority < playbookMinPriority || req.Priority > playbookMaxPriority {
		return errors.New("priority must be between -1000 and 1000")
	}
	return nil
}
