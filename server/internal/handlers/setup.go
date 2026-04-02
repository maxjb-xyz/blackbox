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
