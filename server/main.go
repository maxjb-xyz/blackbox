package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/db"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/middleware"
	"blackbox/server/internal/static"
	"github.com/go-chi/chi/v5"
)

//go:embed web/dist
var staticFiles embed.FS

var (
	Version = "dev"
	Commit  = "unknown"
)

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

	dbPath := getEnv("DB_PATH", "/data/lablog.db")
	database, err := db.Init(dbPath)
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	log.Printf("database initialized at %s", dbPath)

	oidcEnabled := os.Getenv("OIDC_ENABLED") == "true"
	var oidcProviderPtr unsafe.Pointer

	if oidcEnabled {
		log.Printf("OIDC enabled, performing discovery against %s", os.Getenv("OIDC_ISSUER"))
		go func() {
			const maxAttempts = 5
			const retryInterval = 10 * time.Second
			for i := 1; i <= maxAttempts; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				provider, err := auth.NewOIDCProvider(
					ctx,
					os.Getenv("OIDC_ISSUER"),
					os.Getenv("OIDC_CLIENT_ID"),
					os.Getenv("OIDC_CLIENT_SECRET"),
					os.Getenv("OIDC_REDIRECT_URL"),
				)
				cancel()
				if err == nil {
					atomic.StorePointer(&oidcProviderPtr, unsafe.Pointer(provider))
					log.Printf("OIDC provider ready")
					return
				}
				log.Printf("OIDC discovery attempt %d/%d failed: %v", i, maxAttempts, err)
				if i < maxAttempts {
					time.Sleep(retryInterval)
				}
			}
			log.Printf("OIDC provider unavailable after %d attempts, routes will return 503", maxAttempts)
		}()
	}

	oidcProvider := func() *auth.OIDCProvider {
		return (*auth.OIDCProvider)(atomic.LoadPointer(&oidcProviderPtr))
	}

	r := chi.NewRouter()
	r.Use(middleware.SecurityHeaders())

	r.Get("/api/setup/status", handlers.SetupStatus(database))
	r.Get("/api/setup/health", func(w http.ResponseWriter, req *http.Request) {
		handlers.HealthCheck(database, oidcEnabled, oidcProvider() != nil)(w, req)
	})
	r.Group(func(r chi.Router) {
		r.Use(middleware.RateLimit(10*time.Minute, 3))
		r.Post("/api/auth/bootstrap", handlers.Bootstrap(database, jwtSecret))
	})
	r.Group(func(r chi.Router) {
		r.Use(middleware.RateLimit(time.Minute, 10))
		r.Post("/api/auth/login", handlers.Login(database, jwtSecret))
		r.Post("/api/auth/register", handlers.Register(database, jwtSecret))
	})
	r.Post("/api/auth/logout", handlers.Logout())

	if oidcEnabled {
		r.Get("/api/auth/oidc/login", func(w http.ResponseWriter, req *http.Request) {
			handlers.OIDCLogin(database, oidcProvider())(w, req)
		})
		r.Get("/api/auth/oidc/callback", func(w http.ResponseWriter, req *http.Request) {
			handlers.OIDCCallback(database, oidcProvider(), jwtSecret)(w, req)
		})
	}

	r.Group(func(r chi.Router) {
		r.Use(middleware.JWTAuth(jwtSecret))
		r.Get("/api/auth/me", handlers.CurrentUser())
		r.Post("/api/auth/invite", handlers.CreateInvite(database))
		r.Get("/api/auth/invite", handlers.ListInvites(database))
		r.Get("/api/nodes", handlers.ListNodes(database))
		r.Get("/api/entries", handlers.ListEntries(database))
		r.Post("/api/entries", handlers.CreateEntry(database))
		r.Get("/api/entries/{id}", handlers.GetEntry(database))
		r.Post("/api/entries/{id}/notes", handlers.CreateNote(database))
		r.Get("/api/entries/{id}/notes", handlers.ListNotes(database))
		r.Delete("/api/notes/{id}", handlers.DeleteNote(database))
		r.Get("/api/services/aliases", handlers.ListServiceAliases(database))
		r.Post("/api/services/aliases", handlers.CreateServiceAlias(database))
		r.Delete("/api/services/aliases/{alias}", handlers.DeleteServiceAlias(database))
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.AgentAuth(agentConfig))
		r.Post("/api/agent/push", handlers.AgentPush(database))
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.WebhookAuth(webhookSecret))
		r.Post("/api/webhooks/uptime", handlers.WebhookUptime(database))
		r.Post("/api/webhooks/watchtower", handlers.WebhookWatchtower(database))
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
