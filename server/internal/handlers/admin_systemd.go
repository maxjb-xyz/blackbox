package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

var validUnitName = regexp.MustCompile(`^[A-Za-z0-9:_.\-@\\]+\.(service|socket|target|timer|mount|automount|path|scope|slice|swap|device)$`)

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
			var cfg struct {
				Units []string `json:"units"`
			}
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

		for _, unit := range clean {
			if !validUnitName.MatchString(unit) {
				writeError(w, http.StatusBadRequest, "invalid unit name: "+unit)
				return
			}
		}

		cfgJSON, err := json.Marshal(map[string]any{"units": clean})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to serialize units")
			return
		}

		err = db.Transaction(func(tx *gorm.DB) error {
			var existing models.DataSourceInstance
			findErr := tx.Where("type = ? AND node_id = ?", "systemd", nodeName).First(&existing).Error
			now := time.Now().UTC()
			if findErr == nil {
				existing.Config = string(cfgJSON)
				existing.UpdatedAt = now
				return tx.Save(&existing).Error
			}
			if !errors.Is(findErr, gorm.ErrRecordNotFound) {
				return findErr
			}
			inst := models.DataSourceInstance{
				ID: ulid.Make().String(), Type: "systemd", Scope: "agent",
				NodeID: &nodeName, Name: "Systemd", Config: string(cfgJSON),
				Enabled: true, CreatedAt: now, UpdatedAt: now,
			}
			return tx.Create(&inst).Error
		})
		if err != nil {
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
