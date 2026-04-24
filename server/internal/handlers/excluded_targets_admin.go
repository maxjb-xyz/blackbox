package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

type excludedTargetRequest struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func ListExcludedTargets(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var targets []models.ExcludedTarget
		if err := database.Order("type ASC").Order("name ASC").Find(&targets).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list excluded targets")
			return
		}
		if targets == nil {
			targets = []models.ExcludedTarget{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(targets); err != nil {
			log.Printf("ListExcludedTargets encode: %v", err)
		}
	}
}

func CreateExcludedTarget(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req excludedTargetRequest
		if !decodeJSONBody(w, r, 1<<20, &req) {
			return
		}
		targetType := strings.ToLower(strings.TrimSpace(req.Type))
		name := strings.ToLower(strings.TrimSpace(req.Name))
		if targetType != "container" && targetType != "stack" {
			writeError(w, http.StatusBadRequest, "type must be container or stack")
			return
		}
		if name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		var existing models.ExcludedTarget
		err := database.Where("type = ? AND lower(name) = ?", targetType, name).First(&existing).Error
		if err == nil {
			writeError(w, http.StatusConflict, "excluded target already exists")
			return
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(w, http.StatusInternalServerError, "failed to check excluded target")
			return
		}

		target := models.ExcludedTarget{
			ID:        ulid.Make().String(),
			Type:      targetType,
			Name:      name,
			CreatedAt: time.Now().UTC(),
		}
		if err := database.Create(&target).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create excluded target")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(target); err != nil {
			log.Printf("CreateExcludedTarget encode: %v", err)
		}
	}
}

func DeleteExcludedTarget(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		result := database.Delete(&models.ExcludedTarget{}, "id = ?", id)
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete excluded target")
			return
		}
		if result.RowsAffected == 0 {
			writeError(w, http.StatusNotFound, "excluded target not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
