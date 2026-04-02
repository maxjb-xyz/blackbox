package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"strings"

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

	agentToken := os.Getenv("AGENT_TOKEN")
	if agentToken == "" {
		log.Fatal("AGENT_TOKEN environment variable is required")
	}

	dbPath := getEnv("DB_PATH", "/data/lablog.db")
	database, err := db.Init(dbPath)
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	log.Printf("database initialized at %s", dbPath)

	r := chi.NewRouter()

	r.Get("/api/setup/status", handlers.SetupStatus(database))
	r.Post("/api/auth/bootstrap", handlers.Bootstrap(database, jwtSecret))
	r.Post("/api/auth/login", handlers.Login(database, jwtSecret))

	if os.Getenv("OIDC_ENABLED") == "true" {
		log.Println("OIDC enabled (stub — full implementation pending)")
		r.Get("/api/auth/oidc/login", handlers.OIDCStub())
		r.Get("/api/auth/oidc/callback", handlers.OIDCStub())
	}

	r.Group(func(r chi.Router) {
		r.Use(middleware.JWTAuth(jwtSecret))
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.AgentAuth(agentToken))
		r.Post("/api/ingest", handlers.Ingest(database))
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
