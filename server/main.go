package main

import (
	"context"
	"embed"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/db"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/hub"
	"blackbox/server/internal/incidents"
	"blackbox/server/internal/middleware"
	"blackbox/server/internal/models"
	"blackbox/server/internal/notify"
	"blackbox/server/internal/static"
	"blackbox/shared/timezone"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

//go:embed web/dist
var staticFiles embed.FS

//go:embed docs/openapi.yaml
var openapiSpec []byte

var (
	Version = "dev"
	Commit  = "unknown"
)

const defaultDBPath = "/data/blackbox.db"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--health-check" {
		addr := getEnv("LISTEN_ADDR", ":8080")
		if strings.HasPrefix(addr, ":") {
			addr = "localhost" + addr
		}
		// Plain HTTP is intentional: TLS termination is expected at the ingress layer.
		// This check targets the container's loopback interface only.
		client := &http.Client{Timeout: 4 * time.Second}
		resp, err := client.Get("http://" + addr + "/api/setup/health")
		if err != nil {
			os.Exit(1)
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			os.Exit(1)
		}
		os.Exit(0)
	}

	if tz, err := timezone.ConfigureLocal(); err != nil {
		log.Printf("timezone: invalid TZ %q: %v; using container default timezone", tz, err)
	}
	log.Printf("Blackbox Server %s (%s) starting", Version, Commit)
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
	db.StartOIDCStateSweeper(rootCtx, database)
	eventHub := hub.New()
	notifier := notify.NewDispatcher(database)
	handlers.StartNodeStatusMonitor(rootCtx, database, eventHub, 0)
	incidentCh := incidents.NewChannel()
	incidentMgr := incidents.NewManager(database, eventHub, notifier)
	managerCtx, stopManager := context.WithCancel(context.Background())
	defer stopManager()
	managerDone := make(chan struct{})
	go func() {
		defer close(managerDone)
		incidentMgr.Run(managerCtx, incidentCh)
	}()

	registry := auth.NewOIDCRegistry(database)

	if os.Getenv("OIDC_ENABLED") == "true" {
		var providerCount int64
		if err := database.Model(&models.OIDCProviderConfig{}).Count(&providerCount).Error; err != nil {
			log.Printf("failed to count OIDC providers for env migration: %v", err)
		} else if providerCount == 0 {
			issuer := strings.TrimSpace(os.Getenv("OIDC_ISSUER"))
			clientID := strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID"))
			clientSecret := strings.TrimSpace(os.Getenv("OIDC_CLIENT_SECRET"))
			redirectURL := strings.TrimSpace(os.Getenv("OIDC_REDIRECT_URL"))
			if issuer == "" || clientID == "" || clientSecret == "" || redirectURL == "" {
				log.Printf("OIDC_ENABLED=true but one or more required env vars are missing; skipping OIDC provider seed")
			} else {
				provider := models.OIDCProviderConfig{
					ID:                   ulid.Make().String(),
					Name:                 "SSO",
					Issuer:               issuer,
					ClientID:             clientID,
					ClientSecret:         clientSecret,
					RedirectURL:          redirectURL,
					RequireVerifiedEmail: models.BoolPtr(true),
					Enabled:              models.BoolPtr(true),
				}
				if err := database.Create(&provider).Error; err != nil {
					log.Printf("failed to seed OIDC provider from env: %v", err)
				} else {
					log.Printf("OIDC_ENABLED env vars detected, seeded 'SSO' provider — migrate to admin UI to manage")
				}
			}
		}
	}

	go func() {
		const retryInterval = 60 * time.Second
		lastOIDCRegistryStatus := oidcRegistryStatusUnknown
		for attempt := 1; ; attempt++ {
			if err := rootCtx.Err(); err != nil {
				return
			}
			ctx, cancel := context.WithTimeout(rootCtx, 30*time.Second)
			err := registry.Reload(ctx)
			cancel()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Printf("OIDC registry reload attempt %d failed: %v", attempt, err)
			} else {
				providers, listErr := registry.ListEnabled()
				if listErr != nil {
					log.Printf("OIDC registry readiness check %d failed: %v", attempt, listErr)
				} else {
					currentOIDCRegistryStatus := oidcRegistryStatusFromProviders(providers, registry)
					if msg := oidcRegistryTransitionMessage(attempt, lastOIDCRegistryStatus, currentOIDCRegistryStatus); msg != "" {
						log.Print(msg)
					}
					lastOIDCRegistryStatus = currentOIDCRegistryStatus
				}
			}
			select {
			case <-rootCtx.Done():
				return
			case <-time.After(retryInterval):
			}
		}
	}()

	r := chi.NewRouter()
	r.Use(middleware.SecurityHeaders())

	r.Get("/api/setup/status", handlers.SetupStatus(database))
	r.Get("/api/setup/health", handlers.HealthCheck(database, registry))
	r.Get("/api/openapi.yaml", handlers.OpenAPISpec(openapiSpec))
	r.Get("/api/docs", handlers.APIDocs())
	r.Group(func(r chi.Router) {
		r.Use(middleware.RateLimit(10*time.Minute, 3))
		r.Post("/api/auth/bootstrap", handlers.Bootstrap(database, jwtSecret))
	})
	r.Get("/api/auth/oidc/providers", handlers.ListPublicOIDCProviders(database, registry))
	r.With(middleware.RateLimit(time.Minute, 10)).Get("/api/auth/oidc/{provider_id}/login", handlers.OIDCProviderLogin(database, registry))
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
		r.Patch("/api/auth/me", handlers.UpdateAccount(database, jwtSecret))
		r.Post("/api/auth/invite", handlers.CreateInvite(database))
		r.Get("/api/auth/invite", handlers.ListInvites(database))
		r.Get("/api/nodes", handlers.ListNodes(database))
		r.Get("/api/incidents", handlers.ListIncidents(database))
		r.Get("/api/incidents/summary", handlers.GetIncidentSummary(database))
		r.Post("/api/incidents/membership", handlers.ListIncidentMembership(database))
		r.Get("/api/incidents/{id}", handlers.GetIncident(database))
		r.Get("/api/incidents/{id}/report.pdf", handlers.DownloadIncidentReport(database))
		r.Get("/api/entries", handlers.ListEntries(database))
		r.Get("/api/entries/services", handlers.ListEntryServices(database))
		r.Post("/api/entries", handlers.CreateEntry(database, eventHub, incidentCh, managerCtx.Done()))
		r.Get("/api/entries/{id}", handlers.GetEntry(database))
		r.Post("/api/entries/{id}/notes", handlers.CreateNote(database))
		r.Get("/api/entries/{id}/notes", handlers.ListNotes(database))
		r.Delete("/api/notes/{id}", handlers.DeleteNote(database))
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
		r.Get("/api/admin/config", handlers.AdminConfig(database, webhookSecret))
		r.Put("/api/admin/settings/file-watcher", handlers.UpdateFileWatcherSettings(database))
		r.Put("/api/admin/settings/base-url", handlers.UpdateBaseURLSetting(database))
		r.Get("/api/admin/settings/systemd", handlers.GetSystemdSettings(database))
		r.Put("/api/admin/settings/systemd/{node_name}", handlers.UpdateSystemdSettings(database))
		r.Put("/api/admin/settings/ai", handlers.UpdateAISettings(database))
		r.Post("/api/admin/settings/ai/test", handlers.TestAISettings(database))
		r.Put("/api/admin/settings/ollama", handlers.UpdateOllamaSettingsLegacy(database)) // deprecated alias
		r.Get("/api/admin/oidc/providers", handlers.ListOIDCProviders(database))
		r.Post("/api/admin/oidc/providers", handlers.CreateOIDCProvider(database, registry))
		r.Patch("/api/admin/oidc/providers/{id}", handlers.UpdateOIDCProvider(database, registry))
		r.Delete("/api/admin/oidc/providers/{id}", handlers.DeleteOIDCProvider(database, registry))
		r.Get("/api/admin/oidc/policy", handlers.GetOIDCPolicy(database))
		r.Put("/api/admin/oidc/policy", handlers.SetOIDCPolicy(database))
		r.Get("/api/admin/notifications", handlers.ListNotificationDests(database))
		r.Post("/api/admin/notifications", handlers.CreateNotificationDest(database))
		r.Put("/api/admin/notifications/{id}", handlers.UpdateNotificationDest(database))
		r.Delete("/api/admin/notifications/{id}", handlers.DeleteNotificationDest(database))
		r.Post("/api/admin/notifications/{id}/test", handlers.TestNotificationDest(database, notifier))
		r.Get("/api/admin/excluded-targets", handlers.ListExcludedTargets(database))
		r.Post("/api/admin/excluded-targets", handlers.CreateExcludedTarget(database))
		r.Delete("/api/admin/excluded-targets/{id}", handlers.DeleteExcludedTarget(database))
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.AgentAuth(agentConfig))
		r.Get("/api/agent/config", handlers.AgentConfig(database))
		r.Post("/api/agent/push", handlers.AgentPush(database, eventHub, incidentCh, managerCtx.Done()))
		r.Post("/api/agent/push/batch", handlers.AgentPushBatch(database, eventHub, incidentCh, managerCtx.Done()))
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.WebhookAuth(webhookSecret))
		r.Post("/api/webhooks/uptime", handlers.WebhookUptime(database, eventHub, incidentCh, managerCtx.Done()))
		r.Post("/api/webhooks/watchtower", handlers.WebhookWatchtower(database, eventHub, incidentCh, managerCtx.Done()))
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
	server := &http.Server{Addr: addr, Handler: r}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-rootCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("server shutdown error: %v", err)
		}
		if !handlers.WaitForIncidentDispatches(35 * time.Second) {
			log.Printf("incidents: timed out waiting for dispatch goroutines to drain")
		}
		close(incidentCh)
		<-managerDone
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
	if rootCtx.Err() != nil {
		<-shutdownDone
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
