package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

func SetupStatus(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var count int64
		if err := database.Model(&models.User{}).Count(&count).Error; err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "{\"error\":\"service unavailable\"}")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"bootstrapped": count > 0})
	}
}

type healthMCPRuntime interface {
	IsRunning() bool
}

type healthConfigSummary struct {
	Configured bool  `json:"configured"`
	Total      int64 `json:"total"`
	Enabled    int64 `json:"enabled"`
}

type healthMCPStatus struct {
	Enabled         bool `json:"enabled"`
	TokenConfigured bool `json:"token_configured"`
	Running         bool `json:"running"`
}

type healthAIStatus struct {
	Provider         string `json:"provider"`
	Configured       bool   `json:"configured"`
	Testable         bool   `json:"testable"`
	APIKeyConfigured bool   `json:"api_key_configured"`
	Mode             string `json:"mode"`
}

type healthNodeSummary struct {
	Total   int64 `json:"total"`
	Online  int64 `json:"online"`
	Offline int64 `json:"offline"`
	Stale   int64 `json:"stale"`
}

type healthResponse struct {
	Version       string              `json:"version"`
	Commit        string              `json:"commit"`
	Database      string              `json:"database"`
	OIDC          string              `json:"oidc"`
	OIDCEnabled   bool                `json:"oidc_enabled"`
	MCP           healthMCPStatus     `json:"mcp"`
	AI            healthAIStatus      `json:"ai"`
	Nodes         healthNodeSummary   `json:"nodes"`
	Notifications healthConfigSummary `json:"notifications"`
	Webhooks      healthConfigSummary `json:"webhooks"`
}

type publicHealthResponse struct {
	Database    string `json:"database"`
	OIDC        string `json:"oidc"`
	OIDCEnabled bool   `json:"oidc_enabled"`
}

// HealthCheck preserves the original handler contract for callers that do not
// have access to build metadata or the live MCP manager.
func HealthCheck(database *gorm.DB, registry *auth.OIDCRegistry) http.HandlerFunc {
	return HealthCheckWithBuildInfo(database, registry, nil, "dev", "unknown")
}

// PublicHealthCheck exposes only setup-time liveness information. Detailed
// operator diagnostics are served by HealthCheckWithBuildInfo behind auth.
func PublicHealthCheck(database *gorm.DB, registry *auth.OIDCRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := publicHealthResponse{
			Database: "ok",
			OIDC:     "disabled",
		}
		if err := database.Exec("SELECT 1").Error; err != nil {
			resp.Database = "error"
			writePublicHealthResponse(w, http.StatusServiceUnavailable, resp)
			return
		}
		oidcStatus, oidcEnabled := oidcHealthStatus(database, registry)
		resp.OIDC = oidcStatus
		resp.OIDCEnabled = oidcEnabled
		writePublicHealthResponse(w, http.StatusOK, resp)
	}
}

// HealthCheckWithBuildInfo returns a bounded, local-only operator health view.
// It reads persisted configuration and in-process state but never contacts an
// OIDC, AI, webhook, notification, or MCP endpoint.
func HealthCheckWithBuildInfo(database *gorm.DB, registry *auth.OIDCRegistry, mcpRuntime healthMCPRuntime, version, commit string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := healthResponse{
			Version:  version,
			Commit:   commit,
			Database: "ok",
			OIDC:     "disabled",
			AI: healthAIStatus{
				Provider: "ollama",
				Mode:     "analysis",
			},
		}
		if mcpRuntime != nil {
			resp.MCP.Running = mcpRuntime.IsRunning()
		}

		dbStatus := "ok"
		if err := database.Exec("SELECT 1").Error; err != nil {
			dbStatus = "error"
		}
		resp.Database = dbStatus
		if dbStatus != "ok" {
			writeHealthResponse(w, http.StatusServiceUnavailable, resp)
			return
		}

		oidcStatus, oidcEnabled := oidcHealthStatus(database, registry)
		resp.OIDC = oidcStatus
		resp.OIDCEnabled = oidcEnabled

		mcpSettings, err := getMCPSettings(database)
		if err != nil {
			log.Printf("health: MCP settings query failed: %v", err)
		} else {
			resp.MCP.Enabled = mcpSettings.enabled
			resp.MCP.TokenConfigured = mcpSettings.tokenSet
		}

		aiSettings, err := getAISettings(database)
		if err != nil {
			log.Printf("health: AI settings query failed: %v", err)
		} else {
			resp.AI.Provider = aiSettings.provider
			resp.AI.APIKeyConfigured = aiSettings.apiKeySet
			resp.AI.Mode = aiSettings.mode
			resp.AI.Configured = strings.TrimSpace(aiSettings.url) != "" && strings.TrimSpace(aiSettings.model) != ""
			resp.AI.Testable = resp.AI.Configured && (aiSettings.provider == "ollama" || aiSettings.provider == "openai_compat")
		}

		var nodeStats struct {
			Total   int64 `gorm:"column:total"`
			Online  int64 `gorm:"column:online"`
			Offline int64 `gorm:"column:offline"`
			Stale   int64 `gorm:"column:stale"`
		}
		cutoff := time.Now().UTC().Add(-nodeOfflineAfter)
		if err := database.Model(&models.Node{}).Select(
			"COUNT(*) AS total, COALESCE(SUM(CASE WHEN status <> ? AND last_seen > ? THEN 1 ELSE 0 END), 0) AS online, COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS offline, COALESCE(SUM(CASE WHEN last_seen <= ? THEN 1 ELSE 0 END), 0) AS stale",
			nodeStatusOffline, cutoff, nodeStatusOffline, cutoff,
		).Scan(&nodeStats).Error; err != nil {
			log.Printf("health: node summary query failed: %v", err)
		} else {
			resp.Nodes = healthNodeSummary{
				Total:   nodeStats.Total,
				Online:  nodeStats.Online,
				Offline: nodeStats.Offline,
				Stale:   nodeStats.Stale,
			}
		}

		resp.Notifications = notificationHealthSummary(database)
		resp.Webhooks = webhookHealthSummary(database)

		writeHealthResponse(w, http.StatusOK, resp)
	}
}

func oidcHealthStatus(database *gorm.DB, registry *auth.OIDCRegistry) (string, bool) {
	var providers []models.OIDCProviderConfig
	if err := database.Where("enabled = ?", true).Limit(64).Find(&providers).Error; err != nil {
		log.Printf("health: OIDC provider query failed: %v", err)
		return "error", true
	}
	if len(providers) == 0 {
		return "disabled", false
	}
	if registry != nil {
		for _, provider := range providers {
			if registry.Get(provider.ID) != nil {
				return "ok", true
			}
		}
	}
	return "unavailable", true
}

func notificationHealthSummary(database *gorm.DB) healthConfigSummary {
	return configHealthSummary(database, &models.NotificationDest{})
}

func webhookHealthSummary(database *gorm.DB) healthConfigSummary {
	var summary healthConfigSummary
	var row struct {
		Total   int64 `gorm:"column:total"`
		Enabled int64 `gorm:"column:enabled"`
	}
	if err := database.Model(&models.DataSourceInstance{}).
		Where("scope = ? AND type LIKE ?", models.ScopeServer, "webhook_%").
		Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN enabled = ? THEN 1 ELSE 0 END), 0) AS enabled", true).
		Scan(&row).Error; err != nil {
		log.Printf("health: webhook summary query failed: %v", err)
		return summary
	}
	summary.Total = row.Total
	summary.Enabled = row.Enabled
	summary.Configured = row.Total > 0
	return summary
}

func configHealthSummary(database *gorm.DB, model interface{}) healthConfigSummary {
	var summary healthConfigSummary
	var row struct {
		Total   int64 `gorm:"column:total"`
		Enabled int64 `gorm:"column:enabled"`
	}
	if err := database.Model(model).
		Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN enabled = ? THEN 1 ELSE 0 END), 0) AS enabled", true).
		Scan(&row).Error; err != nil {
		log.Printf("health: configuration summary query failed: %v", err)
		return summary
	}
	summary.Total = row.Total
	summary.Enabled = row.Enabled
	summary.Configured = row.Total > 0
	return summary
}

func writeHealthResponse(w http.ResponseWriter, status int, resp healthResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func writePublicHealthResponse(w http.ResponseWriter, status int, resp publicHealthResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}
