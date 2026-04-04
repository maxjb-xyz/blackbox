package handlers

import (
	"encoding/json"
	"net/http"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

func SetupStatus(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var count int64
		database.Model(&models.User{}).Count(&count)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"bootstrapped": count > 0})
	}
}

func HealthCheck(database *gorm.DB, registry *auth.OIDCRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbStatus := "ok"
		if err := database.Exec("SELECT 1").Error; err != nil {
			dbStatus = "error"
		}

		oidcStatus := "disabled"
		oidcEnabled := false
		var providers []models.OIDCProviderConfig
		if err := database.Where("enabled = ?", true).Find(&providers).Error; err == nil && len(providers) > 0 {
			oidcEnabled = true
			oidcStatus = "unavailable"
			if registry != nil {
				for _, provider := range providers {
					if registry.Get(provider.ID) != nil {
						oidcStatus = "ok"
						break
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"database":     dbStatus,
			"oidc":         oidcStatus,
			"oidc_enabled": oidcEnabled,
		})
	}
}
