package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"blackbox/agent/internal/client"
	"blackbox/agent/internal/docker"
	"blackbox/agent/internal/files"
	"blackbox/agent/internal/sender"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
)

var (
	Version = "dev"
	Commit  = "unknown"
)

func main() {
	log.Printf("Blackbox Agent %s (%s) starting", Version, Commit)

	serverURL := mustEnv("SERVER_URL")
	agentToken := mustEnv("AGENT_TOKEN")

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		var err error
		nodeName, err = os.Hostname()
		if err != nil {
			nodeName = "unknown"
		}
	}

	watchPaths := splitEnv("WATCH_PATHS")
	watchIgnore := splitEnv("WATCH_IGNORE")

	c := client.New(serverURL, agentToken)
	s := sender.New(c)
	out := s.Chan()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.Start(ctx)
	go docker.Watch(ctx, nodeName, out)

	if len(watchPaths) > 0 {
		count := files.Watch(ctx, nodeName, watchPaths, watchIgnore, out)
		log.Printf("file watcher: watching %d directories across %d root paths", count, len(watchPaths))
		if count == 0 {
			log.Printf("file watcher: WARNING — no directories registered; check WATCH_PATHS and system max_user_watches")
		}
	} else {
		log.Println("file watcher: WATCH_PATHS not set, file watching disabled")
	}

	out <- types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  nodeName,
		Source:    "agent",
		Event:     "start",
		Content:   "Blackbox Agent started",
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				out <- types.Entry{
					ID:        ulid.Make().String(),
					Timestamp: time.Now().UTC(),
					NodeName:  nodeName,
					Source:    "agent",
					Event:     "heartbeat",
					Content:   "Blackbox Agent heartbeat",
				}
			}
		}
	}()

	log.Printf("startup heartbeat sent (node: %s)", nodeName)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	cancel()

	log.Println("waiting for sender to drain queued events...")
	<-s.Done()
	log.Println("shutdown complete")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s environment variable is required", key)
	}
	return v
}

func splitEnv(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ":")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}