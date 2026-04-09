package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

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

func GetIncidentSummary(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var summary struct {
			OpenCount          int64 `gorm:"column:open_count"`
			ConfirmedOpenCount int64 `gorm:"column:confirmed_open_count"`
		}
		if err := database.Model(&models.Incident{}).
			Select(`
				COALESCE(SUM(CASE WHEN status = 'open' THEN 1 ELSE 0 END), 0) AS open_count,
				COALESCE(SUM(CASE WHEN status = 'open' AND confidence = 'confirmed' THEN 1 ELSE 0 END), 0) AS confirmed_open_count
			`).
			Scan(&summary).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch incident summary")
			return
		}

		resp := struct {
			OpenCount        int64 `json:"open_count"`
			HasConfirmedOpen bool  `json:"has_confirmed_open"`
		}{
			OpenCount:        summary.OpenCount,
			HasConfirmedOpen: summary.ConfirmedOpenCount > 0,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("GetIncidentSummary encode: %v", err)
		}
	}
}

type incidentEntryDetail struct {
	Link models.IncidentEntry `json:"link"`
	Data *types.Entry         `json:"entry"`
}

type incidentMembership struct {
	ID         string `json:"id"`
	Confidence string `json:"confidence"`
}

type incidentMembershipRow struct {
	EntryID    string    `gorm:"column:entry_id"`
	IncidentID string    `gorm:"column:incident_id"`
	Confidence string    `gorm:"column:confidence"`
	OpenedAt   time.Time `gorm:"column:opened_at"`
}

func ListIncidentMembership(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			EntryIDs []string `json:"entry_ids"`
		}
		if !decodeJSONBody(w, r, 32<<10, &req) {
			return
		}

		if len(req.EntryIDs) > 200 {
			writeError(w, http.StatusBadRequest, "entry_ids must contain at most 200 IDs")
			return
		}

		entryIDs := make([]string, 0, len(req.EntryIDs))
		seen := make(map[string]struct{}, len(req.EntryIDs))
		for _, entryID := range req.EntryIDs {
			entryID = strings.TrimSpace(entryID)
			if entryID == "" {
				continue
			}
			if _, ok := seen[entryID]; ok {
				continue
			}
			seen[entryID] = struct{}{}
			entryIDs = append(entryIDs, entryID)
		}

		resp := struct {
			Memberships map[string]incidentMembership `json:"memberships"`
		}{
			Memberships: map[string]incidentMembership{},
		}
		if len(entryIDs) == 0 {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				log.Printf("ListIncidentMembership encode empty: %v", err)
			}
			return
		}

		var rows []incidentMembershipRow
		if err := database.Table("incident_entries AS ie").
			Select("ie.entry_id, incidents.id AS incident_id, incidents.confidence, incidents.opened_at").
			Joins("JOIN incidents ON incidents.id = ie.incident_id").
			Where("ie.entry_id IN ? AND incidents.status = ?", entryIDs, "open").
			Order("incidents.opened_at DESC").
			Scan(&rows).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch incident membership")
			return
		}

		for _, row := range rows {
			if _, ok := resp.Memberships[row.EntryID]; ok {
				continue
			}
			resp.Memberships[row.EntryID] = incidentMembership{
				ID:         row.IncidentID,
				Confidence: row.Confidence,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("ListIncidentMembership encode: %v", err)
		}
	}
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
				log.Printf("GetIncident missing entry for incident %s entry %s role %s: %v", link.IncidentID, link.EntryID, link.Role, err)
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
