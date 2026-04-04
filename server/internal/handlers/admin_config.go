package handlers

import (
	"encoding/json"
	"log"
	"net/http"
)

func AdminConfig(webhookSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"webhook_secret": webhookSecret,
		}); err != nil {
			log.Printf("AdminConfig encode: %v", err)
		}
	}
}
