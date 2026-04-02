package main

import (
	"log"
	"os"
	"time"

	"blackbox/agent/internal/client"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
)

var (
	Version = "dev"
	Commit  = "unknown"
)

func main() {
	log.Printf("Blackbox Agent %s (%s) starting", Version, Commit)

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		log.Fatal("SERVER_URL environment variable is required")
	}

	agentToken := os.Getenv("AGENT_TOKEN")
	if agentToken == "" {
		log.Fatal("AGENT_TOKEN environment variable is required")
	}

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		var err error
		nodeName, err = os.Hostname()
		if err != nil {
			nodeName = "unknown"
		}
	}

	c := client.New(serverURL, agentToken)

	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  nodeName,
		Source:    "agent",
		Event:     "start",
		Content:   "Blackbox Agent started",
	}
	if err := c.Send(entry); err != nil {
		log.Printf("warning: failed to send startup heartbeat: %v", err)
	} else {
		log.Printf("startup heartbeat sent (node: %s)", nodeName)
	}

	log.Println("agent running — watchers not yet implemented")
	select {}
}
