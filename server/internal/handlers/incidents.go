package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func ListIncidents(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		status := q.Get("status")
		confidence := q.Get("confidence")
		service := q.Get("service")
		limitStr := q.Get("limit")

		limit := 50
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
			limit = n
		}

		tx := database.Model(&models.Incident{}).Order("opened_at DESC").Limit(limit + 1)
		if status != "" {
			tx = tx.Where("status = ?", status)
		}
		if confidence != "" {
			tx = tx.Where("confidence = ?", confidence)
		}
		if service != "" {
			escapedService := strings.NewReplacer(
				"\\", "\\\\",
				"%", "\\%",
				"_", "\\_",
			).Replace(service)
			tx = tx.Where(`services LIKE ? ESCAPE '\'`, "%\""+escapedService+"\"%")
		}

		var incidents []models.Incident
		if err := tx.Find(&incidents).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch incidents")
			return
		}

		type response struct {
			Incidents []models.Incident `json:"incidents"`
			HasMore   bool              `json:"has_more"`
		}

		resp := response{Incidents: incidents}
		if len(incidents) > limit {
			resp.Incidents = incidents[:limit]
			resp.HasMore = true
		}
		if resp.Incidents == nil {
			resp.Incidents = []models.Incident{}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("ListIncidents encode: %v", err)
		}
	}
}

type incidentEntryDetail struct {
	Link models.IncidentEntry `json:"link"`
	Data *types.Entry         `json:"entry"`
}

func GetIncident(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var incident models.Incident
		if err := database.First(&incident, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusNotFound, "incident not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to fetch incident")
			return
		}

		var links []models.IncidentEntry
		if err := database.Where("incident_id = ?", id).
			Order("score DESC").Find(&links).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch incident entries")
			return
		}

		details := make([]incidentEntryDetail, 0, len(links))
		for _, link := range links {
			var entry types.Entry
			if err := database.First(&entry, "id = ?", link.EntryID).Error; err != nil {
				continue
			}
			details = append(details, incidentEntryDetail{Link: link, Data: &entry})
		}

		type response struct {
			Incident models.Incident       `json:"incident"`
			Entries  []incidentEntryDetail `json:"entries"`
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response{Incident: incident, Entries: details}); err != nil {
			log.Printf("GetIncident encode: %v", err)
		}
	}
}
