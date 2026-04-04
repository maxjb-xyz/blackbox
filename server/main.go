package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/db"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/hub"
	"blackbox/server/internal/middleware"
	"blackbox/server/internal/models"
	"blackbox/server/internal/static"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

//go:embed web/dist
var staticFiles embed.FS

var (
	Version = "dev"
	Commit  = "unknown"
)

const defaultDBPath = "/data/blackbox.db"

func main() {
	log.Printf("Blackbox Server %s (%s) starting", Version, Commit)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}

	agentConfig, err := middleware.NewAgentAuthConfig(os.Getenv("AGENT_TOKENS"))
	if err != nil {
		log.Fatalf("AGENT_TOKENS configuration error: %v", err)
	}

	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	if webhookSecret == "" {
		log.Fatal("WEBHOOK_SECRET environment variable is required")
	}

	dbPath := getEnv("DB_PATH", defaultDBPath)
	database, err := db.Init(dbPath)
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	log.Printf("database initialized at %s", dbPath)
	eventHub := hub.New()

	registry := auth.NewOIDCRegistry(database)

	if os.Getenv("OIDC_ENABLED") == "true" {
		var providerCount int64
		if err := database.Model(&models.OIDCProviderConfig{}).Count(&providerCount).Error; err != nil {
			log.Printf("failed to count OIDC providers for env migration: %v", err)
		} else if providerCount == 0 {
			provider := models.OIDCProviderConfig{
				ID:           ulid.Make().String(),
				Name:         "SSO",
				Issuer:       os.Getenv("OIDC_ISSUER"),
				ClientID:     os.Getenv("OIDC_CLIENT_ID"),
				ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
				RedirectURL:  os.Getenv("OIDC_REDIRECT_URL"),
				Enabled:      true,
			}
			if err := database.Create(&provider).Error; err != nil {
				log.Printf("failed to seed OIDC provider from env: %v", err)
			} else {
				log.Printf("OIDC_ENABLED env vars detected, seeded 'SSO' provider — migrate to admin UI to manage")
			}
		}
	}

	go func() {
		const maxAttempts = 5
		const retryInterval = 10 * time.Second
		for i := 1; i <= maxAttempts; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := registry.Reload(ctx)
			cancel()
			if err != nil {
				log.Printf("OIDC registry reload attempt %d/%d failed: %v", i, maxAttempts, err)
			} else {
				providers, listErr := registry.ListEnabled()
				if listErr != nil {
					log.Printf("OIDC registry readiness check %d/%d failed: %v", i, maxAttempts, listErr)
				} else {
					live := len(providers) == 0
					for _, provider := range providers {
						if registry.Get(provider.ID) != nil {
							live = true
							break
						}
					}
					if live {
						if len(providers) > 0 {
							log.Printf("OIDC registry ready")
						}
						return
					}
					log.Printf("OIDC registry reload attempt %d/%d completed but no providers are currently available", i, maxAttempts)
				}
			}
			if i < maxAttempts {
				time.Sleep(retryInterval)
			}
		}
		log.Printf("OIDC providers unavailable after %d attempts, OIDC routes will return 503 until reload succeeds", maxAttempts)
	}()

	r := chi.NewRouter()
	r.Use(middleware.SecurityHeaders())

	r.Get("/api/setup/status", handlers.SetupStatus(database))
	r.Get("/api/setup/health", handlers.HealthCheck(database, registry))
	r.Group(func(r chi.Router) {
		r.Use(middleware.RateLimit(10*time.Minute, 3))
		r.Post("/api/auth/bootstrap", handlers.Bootstrap(database, jwtSecret))
	})
	r.Get("/api/auth/oidc/providers", handlers.ListPublicOIDCProviders(database))
	r.Get("/api/auth/oidc/{provider_id}/login", handlers.OIDCProviderLogin(database, registry))
	r.Get("/api/auth/oidc/{provider_id}/callback", handlers.OIDCProviderCallback(database, registry, jwtSecret))
	r.Group(func(r chi.Router) {
		r.Use(middleware.RateLimit(time.Minute, 10))
		r.Post("/api/auth/login", handlers.Login(database, jwtSecret))
		r.Post("/api/auth/register", handlers.Register(database, jwtSecret))
	})
	r.Post("/api/auth/logout", handlers.Logout())

	r.Group(func(r chi.Router) {
		r.Use(middleware.JWTAuth(jwtSecret))
		r.Use(middleware.TokenVersionCheck(database))
		r.Get("/api/auth/me", handlers.CurrentUser())
		r.Post("/api/auth/invite", handlers.CreateInvite(database))
		r.Get("/api/auth/invite", handlers.ListInvites(database))
		r.Get("/api/nodes", handlers.ListNodes(database))
		r.Get("/api/entries", handlers.ListEntries(database))
		r.Get("/api/entries/services", handlers.ListEntryServices(database))
		r.Post("/api/entries", handlers.CreateEntry(database, eventHub))
		r.Get("/api/entries/{id}", handlers.GetEntry(database))
		r.Post("/api/entries/{id}/notes", handlers.CreateNote(database))
		r.Get("/api/entries/{id}/notes", handlers.ListNotes(database))
		r.Delete("/api/notes/{id}", handlers.DeleteNote(database))
		r.Get("/api/services/aliases", handlers.ListServiceAliases(database))
		r.Post("/api/services/aliases", handlers.CreateServiceAlias(database))
		r.Delete("/api/services/aliases/{alias}", handlers.DeleteServiceAlias(database))
		r.Get("/api/ws", handlers.WebSocketHandler(eventHub))
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.JWTAuth(jwtSecret))
		r.Use(middleware.TokenVersionCheck(database))
		r.Use(middleware.RequireAdmin())
		r.Delete("/api/auth/invite/{id}", handlers.RevokeInvite(database))
		r.Get("/api/admin/users", handlers.ListAdminUsers(database))
		r.Patch("/api/admin/users/{id}", handlers.UpdateAdminUser(database, eventHub))
		r.Post("/api/admin/users/{id}/force-logout", handlers.ForceLogoutUser(database, eventHub))
		r.Delete("/api/admin/users/{id}", handlers.DeleteAdminUser(database, eventHub))
		r.Get("/api/admin/config", handlers.AdminConfig(webhookSecret))
		r.Get("/api/admin/oidc/providers", handlers.ListOIDCProviders(database))
		r.Post("/api/admin/oidc/providers", handlers.CreateOIDCProvider(database, registry))
		r.Patch("/api/admin/oidc/providers/{id}", handlers.UpdateOIDCProvider(database, registry))
		r.Delete("/api/admin/oidc/providers/{id}", handlers.DeleteOIDCProvider(database, registry))
		r.Get("/api/admin/oidc/policy", handlers.GetOIDCPolicy(database))
		r.Put("/api/admin/oidc/policy", handlers.SetOIDCPolicy(database))
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.AgentAuth(agentConfig))
		r.Post("/api/agent/push", handlers.AgentPush(database, eventHub))
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.WebhookAuth(webhookSecret))
		r.Post("/api/webhooks/uptime", handlers.WebhookUptime(database, eventHub))
		r.Post("/api/webhooks/watchtower", handlers.WebhookWatchtower(database, eventHub))
	})

	spaHandler := static.Handler(staticFiles)
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			http.NotFound(w, req)
			return
		}
		spaHandler.ServeHTTP(w, req)
	})
	r.Handle("/*", spaHandler)

	addr := getEnv("LISTEN_ADDR", ":8080")
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
