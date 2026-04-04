package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"blackbox/server/internal/models"
	"blackbox/server/internal/services"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type createServiceAliasRequest struct {
	Canonical string `json:"canonical"`
	Alias     string `json:"alias"`
}

func ListServiceAliases(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var aliases []models.ServiceAlias
		if err := database.Order("canonical ASC, alias ASC").Find(&aliases).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch service aliases")
			return
		}
		if aliases == nil {
			aliases = []models.ServiceAlias{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(aliases)
	}
}

func CreateServiceAlias(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createServiceAliasRequest
		if !decodeJSONBody(w, r, 1<<20, &req) {
			return
		}

		canonical := strings.TrimSpace(req.Canonical)
		alias := strings.TrimSpace(req.Alias)
		if canonical == "" || alias == "" {
			writeError(w, http.StatusBadRequest, "canonical and alias are required")
			return
		}
		normalizedCanonical, err := services.NormalizeService(database, canonical)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to normalize service alias")
			return
		}

		record := models.ServiceAlias{
			Canonical: normalizedCanonical,
			Alias:     alias,
		}
		if err := database.Create(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				writeError(w, http.StatusConflict, "failed to create service alias")
			} else {
				writeError(w, http.StatusInternalServerError, "failed to create service alias")
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(record)
	}
}

func DeleteServiceAlias(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		alias, err := url.PathUnescape(chi.URLParam(r, "alias"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid alias")
			return
		}
		alias = strings.TrimSpace(alias)
		if alias == "" {
			writeError(w, http.StatusBadRequest, "alias is required")
			return
		}

		result := database.Delete(&models.ServiceAlias{}, "alias = ?", alias)
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete service alias")
			return
		}
		if result.RowsAffected == 0 {
			writeError(w, http.StatusNotFound, "service alias not found")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
