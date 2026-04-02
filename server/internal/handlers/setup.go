package handlers

import (
	"encoding/json"
	"net/http"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

func SetupStatus(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var count int64
		database.Model(&models.User{}).Count(&count)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"bootstrapped": count > 0})
	}
}

func HealthCheck(database *gorm.DB, oidcEnabled bool, oidcReady bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbStatus := "ok"
		if err := database.Exec("SELECT 1").Error; err != nil {
			dbStatus = "error"
		}

		oidcStatus := "disabled"
		if oidcEnabled {
			if oidcReady {
				oidcStatus = "ok"
			} else {
				oidcStatus = "unavailable"
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"database":     dbStatus,
			"oidc":         oidcStatus,
			"oidc_enabled": oidcEnabled,
		})
	}
}
