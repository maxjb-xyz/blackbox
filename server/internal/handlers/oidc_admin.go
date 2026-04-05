package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
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

var oidcProviderIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func getOIDCPolicy(db *gorm.DB) (string, error) {
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "oidc_policy").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "open", nil
		}
		return "", err
	}
	switch setting.Value {
	case "open", "existing_only", "invite_required":
		return setting.Value, nil
	default:
		return "", errors.New("invalid OIDC policy value")
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
			ID           string `json:"id"`
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
		req.ID = strings.TrimSpace(req.ID)
		req.Name = strings.TrimSpace(req.Name)
		req.Issuer = strings.TrimSpace(req.Issuer)
		req.ClientID = strings.TrimSpace(req.ClientID)
		req.RedirectURL = strings.TrimSpace(req.RedirectURL)
		if req.Name == "" || req.Issuer == "" || req.ClientID == "" || req.ClientSecret == "" || req.RedirectURL == "" {
			writeError(w, http.StatusBadRequest, "name, issuer, client_id, client_secret, and redirect_url required")
			return
		}
		if req.ID == "" {
			req.ID = ulid.Make().String()
		}
		if !oidcProviderIDPattern.MatchString(req.ID) {
			writeError(w, http.StatusBadRequest, "invalid provider id")
			return
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		provider := models.OIDCProviderConfig{
			ID:           req.ID,
			Name:         req.Name,
			Issuer:       req.Issuer,
			ClientID:     req.ClientID,
			ClientSecret: req.ClientSecret,
			RedirectURL:  req.RedirectURL,
			Enabled:      models.BoolPtr(enabled),
		}
		if err := db.Create(&provider).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create OIDC provider")
			return
		}

		if err := reloadOIDCRegistry(r.Context(), registry); err != nil {
			writeError(w, http.StatusInternalServerError, "OIDC provider saved but registry reload failed")
			return
		}

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
			ID           *string `json:"id"`
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
		if req.ID != nil {
			trimmed := strings.TrimSpace(*req.ID)
			if trimmed == "" {
				writeError(w, http.StatusBadRequest, "id cannot be empty")
				return
			}
			if !oidcProviderIDPattern.MatchString(trimmed) {
				writeError(w, http.StatusBadRequest, "invalid provider id")
				return
			}
			req.ID = &trimmed
		}
		if req.Name != nil {
			trimmed := strings.TrimSpace(*req.Name)
			req.Name = &trimmed
		}
		if req.Name != nil && *req.Name == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		if req.Issuer != nil {
			trimmed := strings.TrimSpace(*req.Issuer)
			req.Issuer = &trimmed
		}
		if req.Issuer != nil && *req.Issuer == "" {
			writeError(w, http.StatusBadRequest, "issuer cannot be empty")
			return
		}
		if req.ClientID != nil {
			trimmed := strings.TrimSpace(*req.ClientID)
			req.ClientID = &trimmed
		}
		if req.ClientID != nil && *req.ClientID == "" {
			writeError(w, http.StatusBadRequest, "client_id cannot be empty")
			return
		}
		if req.RedirectURL != nil {
			trimmed := strings.TrimSpace(*req.RedirectURL)
			req.RedirectURL = &trimmed
		}
		if req.RedirectURL != nil && *req.RedirectURL == "" {
			writeError(w, http.StatusBadRequest, "redirect_url cannot be empty")
			return
		}

		updates := map[string]interface{}{}
		if req.ID != nil {
			updates["id"] = *req.ID
		}
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

		updatedProviderID := providerID
		if req.ID != nil {
			updatedProviderID = *req.ID
		}
		if len(updates) > 0 {
			if err := db.Model(&provider).Updates(updates).Error; err != nil {
				writeError(w, http.StatusInternalServerError, "failed to update OIDC provider")
				return
			}
			if err := db.First(&provider, "id = ?", updatedProviderID).Error; err != nil {
				writeError(w, http.StatusInternalServerError, "failed to fetch updated OIDC provider")
				return
			}
		}

		if err := reloadOIDCRegistry(r.Context(), registry); err != nil {
			writeError(w, http.StatusInternalServerError, "OIDC provider saved but registry reload failed")
			return
		}

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

		if err := reloadOIDCRegistry(r.Context(), registry); err != nil {
			writeError(w, http.StatusInternalServerError, "OIDC provider deleted but registry reload failed")
			return
		}

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

		policy, err := getOIDCPolicy(db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load OIDC policy")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"policy": policy})
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

func ListPublicOIDCProviders(db *gorm.DB, registry *auth.OIDCRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var providers []models.OIDCProviderConfig
		if err := db.Where("enabled = ?", true).Order("name ASC").Find(&providers).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list OIDC providers")
			return
		}
		if len(providers) == 0 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string][]publicOIDCProviderResponse{"providers": []publicOIDCProviderResponse{}})
			return
		}
		if registry == nil || !registry.IsReady() {
			writeError(w, http.StatusServiceUnavailable, "OIDC providers unavailable")
			return
		}

		resp := make([]publicOIDCProviderResponse, 0, len(providers))
		for _, provider := range providers {
			if registry.Get(provider.ID) == nil {
				continue
			}
			resp = append(resp, publicOIDCProviderResponse{
				ID:   provider.ID,
				Name: provider.Name,
			})
		}
		if len(resp) == 0 {
			writeError(w, http.StatusServiceUnavailable, "OIDC providers unavailable")
			return
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
		Enabled:      provider.Enabled != nil && *provider.Enabled,
		CreatedAt:    provider.CreatedAt,
		UpdatedAt:    provider.UpdatedAt,
	}
}

func reloadOIDCRegistry(parent context.Context, registry *auth.OIDCRegistry) error {
	if registry == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	return registry.Reload(ctx)
}
