package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func GetSystemdSettings(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var configs []models.SystemdUnitConfig
		if err := db.Find(&configs).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load systemd settings")
			return
		}

		result := make(map[string][]string, len(configs))
		for _, c := range configs {
			var units []string
			if err := json.Unmarshal([]byte(c.Units), &units); err != nil {
				units = []string{}
			}
			result[c.NodeName] = units
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			log.Printf("GetSystemdSettings encode: %v", err)
		}
	}
}

func UpdateSystemdSettings(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeName := strings.TrimSpace(chi.URLParam(r, "node_name"))
		if nodeName == "" {
			writeError(w, http.StatusBadRequest, "node_name is required")
			return
		}

		var req struct {
			Units []string `json:"units"`
		}
		if !decodeJSONBody(w, r, 64<<10, &req) {
			return
		}
		if req.Units == nil {
			req.Units = []string{}
		}

		clean := make([]string, 0, len(req.Units))
		for _, u := range req.Units {
			t := strings.TrimSpace(u)
			if t == "" {
				continue
			}
			if !hasUnitTypeSuffix(t) {
				t = t + ".service"
			}
			clean = append(clean, t)
		}

		unitsJSON, err := json.Marshal(clean)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encode units")
			return
		}

		config := models.SystemdUnitConfig{
			NodeName: nodeName,
			Units:    string(unitsJSON),
		}
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "node_name"}},
			DoUpdates: clause.AssignmentColumns([]string{"units"}),
		}).Create(&config).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save systemd settings")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

var unitTypeSuffixes = []string{
	".service", ".socket", ".device", ".mount", ".automount",
	".swap", ".target", ".path", ".timer", ".slice", ".scope",
}

func hasUnitTypeSuffix(name string) bool {
	for _, s := range unitTypeSuffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}
