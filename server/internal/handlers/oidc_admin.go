package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type oidcProviderResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Issuer       string    `json:"issuer"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	RedirectURL  string    `json:"redirect_url"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type publicOIDCProviderResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func getOIDCPolicy(db *gorm.DB) string {
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "oidc_policy").Error; err != nil {
		return "open"
	}
	switch setting.Value {
	case "open", "existing_only", "invite_required":
		return setting.Value
	default:
		return "open"
	}
}

func ListOIDCProviders(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok || !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}

		var providers []models.OIDCProviderConfig
		if err := db.Order("created_at ASC").Find(&providers).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list OIDC providers")
			return
		}

		resp := make([]oidcProviderResponse, len(providers))
		for i, provider := range providers {
			resp[i] = toOIDCProviderResponse(provider)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func CreateOIDCProvider(db *gorm.DB, registry *auth.OIDCRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok || !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}

		var req struct {
			Name         string `json:"name"`
			Issuer       string `json:"issuer"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			RedirectURL  string `json:"redirect_url"`
			Enabled      *bool  `json:"enabled"`
		}
		if !decodeJSONBody(w, r, 8<<10, &req) {
			return
		}
		if req.Name == "" || req.Issuer == "" || req.ClientID == "" || req.ClientSecret == "" || req.RedirectURL == "" {
			writeError(w, http.StatusBadRequest, "name, issuer, client_id, client_secret, and redirect_url required")
			return
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		provider := models.OIDCProviderConfig{
			ID:           ulid.Make().String(),
			Name:         req.Name,
			Issuer:       req.Issuer,
			ClientID:     req.ClientID,
			ClientSecret: req.ClientSecret,
			RedirectURL:  req.RedirectURL,
			Enabled:      enabled,
		}
		if err := db.Create(&provider).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create OIDC provider")
			return
		}

		reloadOIDCRegistryAsync(registry)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(toOIDCProviderResponse(provider))
	}
}

func UpdateOIDCProvider(db *gorm.DB, registry *auth.OIDCRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok || !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}

		providerID := chi.URLParam(r, "id")
		var provider models.OIDCProviderConfig
		if err := db.First(&provider, "id = ?", providerID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusNotFound, "OIDC provider not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to fetch OIDC provider")
			return
		}

		var req struct {
			Name         *string `json:"name"`
			Issuer       *string `json:"issuer"`
			ClientID     *string `json:"client_id"`
			ClientSecret *string `json:"client_secret"`
			RedirectURL  *string `json:"redirect_url"`
			Enabled      *bool   `json:"enabled"`
		}
		if !decodeJSONBody(w, r, 8<<10, &req) {
			return
		}
		if req.Name != nil && *req.Name == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		if req.Issuer != nil && *req.Issuer == "" {
			writeError(w, http.StatusBadRequest, "issuer cannot be empty")
			return
		}
		if req.ClientID != nil && *req.ClientID == "" {
			writeError(w, http.StatusBadRequest, "client_id cannot be empty")
			return
		}
		if req.RedirectURL != nil && *req.RedirectURL == "" {
			writeError(w, http.StatusBadRequest, "redirect_url cannot be empty")
			return
		}

		updates := map[string]interface{}{}
		if req.Name != nil {
			updates["name"] = *req.Name
		}
		if req.Issuer != nil {
			updates["issuer"] = *req.Issuer
		}
		if req.ClientID != nil {
			updates["client_id"] = *req.ClientID
		}
		if req.ClientSecret != nil && *req.ClientSecret != "" && *req.ClientSecret != "***" {
			updates["client_secret"] = *req.ClientSecret
		}
		if req.RedirectURL != nil {
			updates["redirect_url"] = *req.RedirectURL
		}
		if req.Enabled != nil {
			updates["enabled"] = *req.Enabled
		}

		if len(updates) > 0 {
			if err := db.Model(&provider).Updates(updates).Error; err != nil {
				writeError(w, http.StatusInternalServerError, "failed to update OIDC provider")
				return
			}
			if err := db.First(&provider, "id = ?", providerID).Error; err != nil {
				writeError(w, http.StatusInternalServerError, "failed to fetch updated OIDC provider")
				return
			}
		}

		reloadOIDCRegistryAsync(registry)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(toOIDCProviderResponse(provider))
	}
}

func DeleteOIDCProvider(db *gorm.DB, registry *auth.OIDCRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok || !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}

		providerID := chi.URLParam(r, "id")
		result := db.Delete(&models.OIDCProviderConfig{}, "id = ?", providerID)
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete OIDC provider")
			return
		}
		if result.RowsAffected == 0 {
			writeError(w, http.StatusNotFound, "OIDC provider not found")
			return
		}

		reloadOIDCRegistryAsync(registry)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

func GetOIDCPolicy(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok || !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"policy": getOIDCPolicy(db)})
	}
}

func SetOIDCPolicy(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok || !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}

		var req struct {
			Policy string `json:"policy"`
		}
		if !decodeJSONBody(w, r, 8<<10, &req) {
			return
		}
		switch req.Policy {
		case "open", "existing_only", "invite_required":
		default:
			writeError(w, http.StatusBadRequest, "invalid OIDC policy")
			return
		}

		setting := models.AppSetting{
			Key:       "oidc_policy",
			Value:     req.Policy,
			UpdatedAt: time.Now(),
		}
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "key"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"value":      req.Policy,
				"updated_at": time.Now(),
			}),
		}).Create(&setting).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update OIDC policy")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"policy": req.Policy})
	}
}

func ListPublicOIDCProviders(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var providers []models.OIDCProviderConfig
		if err := db.Where("enabled = ?", true).Order("name ASC").Find(&providers).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list OIDC providers")
			return
		}

		resp := make([]publicOIDCProviderResponse, len(providers))
		for i, provider := range providers {
			resp[i] = publicOIDCProviderResponse{
				ID:   provider.ID,
				Name: provider.Name,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string][]publicOIDCProviderResponse{"providers": resp})
	}
}

func toOIDCProviderResponse(provider models.OIDCProviderConfig) oidcProviderResponse {
	return oidcProviderResponse{
		ID:           provider.ID,
		Name:         provider.Name,
		Issuer:       provider.Issuer,
		ClientID:     provider.ClientID,
		ClientSecret: "***",
		RedirectURL:  provider.RedirectURL,
		Enabled:      provider.Enabled,
		CreatedAt:    provider.CreatedAt,
		UpdatedAt:    provider.UpdatedAt,
	}
}

func reloadOIDCRegistryAsync(registry *auth.OIDCRegistry) {
	if registry == nil {
		return
	}

	go func() {
		if err := registry.Reload(context.Background()); err != nil {
			log.Printf("oidc registry reload failed: %v", err)
		}
	}()
}
