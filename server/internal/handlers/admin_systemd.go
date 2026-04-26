package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

func GetSystemdSettings(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var instances []models.DataSourceInstance
		if err := db.Where("type = ?", "systemd").Find(&instances).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load systemd settings")
			return
		}

		result := make(map[string][]string)
		for _, inst := range instances {
			if inst.NodeID == nil {
				continue
			}
			var cfg struct{ Units []string `json:"units"` }
			if err := json.Unmarshal([]byte(inst.Config), &cfg); err != nil {
				cfg.Units = []string{}
			}
			if cfg.Units == nil {
				cfg.Units = []string{}
			}
			result[*inst.NodeID] = cfg.Units
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

		seen := make(map[string]struct{}, len(req.Units))
		clean := make([]string, 0, len(req.Units))
		for _, u := range req.Units {
			t := strings.TrimSpace(u)
			if t == "" {
				continue
			}
			if !hasUnitTypeSuffix(t) {
				t = t + ".service"
			}
			if _, dup := seen[t]; dup {
				continue
			}
			seen[t] = struct{}{}
			clean = append(clean, t)
		}

		cfgJSON, err := json.Marshal(map[string]any{"units": clean})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to serialize units")
			return
		}

		var existing models.DataSourceInstance
		err = db.Where("type = ? AND node_id = ?", "systemd", nodeName).First(&existing).Error
		now := time.Now().UTC()
		if err == nil {
			existing.Config = string(cfgJSON)
			existing.UpdatedAt = now
			if err := db.Save(&existing).Error; err != nil {
				writeError(w, http.StatusInternalServerError, "failed to save systemd settings")
				return
			}
		} else {
			inst := models.DataSourceInstance{
				ID: ulid.Make().String(), Type: "systemd", Scope: "agent",
				NodeID: &nodeName, Name: "Systemd", Config: string(cfgJSON),
				Enabled: true, CreatedAt: now, UpdatedAt: now,
			}
			if err := db.Create(&inst).Error; err != nil {
				writeError(w, http.StatusInternalServerError, "failed to save systemd settings")
				return
			}
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
