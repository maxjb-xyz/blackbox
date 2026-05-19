package mcp

import (
	"context"
	"errors"
	"net/http"
	"sync"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

// MCPManager manages mounted MCP runtime state for the main HTTP server.
type MCPManager struct {
	mu      sync.RWMutex
	enabled bool
	token   string
	handler http.Handler
}

func NewMCPManager(db *gorm.DB) *MCPManager {
	srv := buildServer(db)
	return &MCPManager{handler: mcpserver.NewStreamableHTTPServer(srv)}
}

func (m *MCPManager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

func (m *MCPManager) ApplySettings(enabled bool, _ int, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if enabled && token == "" {
		return errors.New("mcp auth token is required")
	}

	m.enabled = enabled
	if enabled {
		m.token = token
		return nil
	}

	m.token = ""
	return nil
}

func (m *MCPManager) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enabled, token, handler := m.snapshot()
		if !enabled {
			writeUnavailable(w)
			return
		}
		if err := ValidateOrigin(r, ""); err != nil {
			writeForbiddenOrigin(w)
			return
		}
		BearerTokenMiddleware(token, handler).ServeHTTP(w, r)
	})
}

func (m *MCPManager) Shutdown(context.Context) error {
	return nil
}

func (m *MCPManager) snapshot() (bool, string, http.Handler) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled, m.token, m.handler
}

func buildServer(db *gorm.DB) *mcpserver.MCPServer {
	srv := mcpserver.NewMCPServer("Blackbox", "1.0.0")
	srv.AddTool(
		mcplib.NewTool("list_incidents",
			mcplib.WithDescription("List Blackbox incidents with optional filters, sorted newest first."),
			mcplib.WithString("status", mcplib.Description("Filter by status."), mcplib.Enum("open", "resolved")),
			mcplib.WithString("confidence", mcplib.Description("Filter by confidence."), mcplib.Enum("confirmed", "suspected")),
			mcplib.WithNumber("limit", mcplib.Description("Maximum incidents to return."), mcplib.Min(1), mcplib.Max(100)),
		),
		handleListIncidents(db),
	)
	srv.AddTool(
		mcplib.NewTool("get_incident",
			mcplib.WithDescription("Get an incident and its linked entries."),
			mcplib.WithString("id", mcplib.Description("Incident ID."), mcplib.Required()),
		),
		handleGetIncident(db),
	)
	srv.AddTool(
		mcplib.NewTool("list_entries",
			mcplib.WithDescription("List timeline entries with optional filters and cursor pagination."),
			mcplib.WithString("node_id", mcplib.Description("Filter by node name.")),
			mcplib.WithString("source", mcplib.Description("Filter by entry source.")),
			mcplib.WithString("event", mcplib.Description("Filter by event type.")),
			mcplib.WithString("since", mcplib.Description("RFC3339 lower timestamp bound.")),
			mcplib.WithString("until", mcplib.Description("RFC3339 upper timestamp bound.")),
			mcplib.WithString("cursor", mcplib.Description("Pagination cursor from a previous response.")),
			mcplib.WithNumber("limit", mcplib.Description("Maximum entries to return."), mcplib.Min(1), mcplib.Max(200)),
		),
		handleListEntries(db),
	)
	srv.AddTool(
		mcplib.NewTool("search_entries",
			mcplib.WithDescription("Search timeline entries with FTS5 and LIKE fallback."),
			mcplib.WithString("query", mcplib.Description("Search query."), mcplib.Required()),
			mcplib.WithString("since", mcplib.Description("Optional RFC3339 lower timestamp bound.")),
			mcplib.WithNumber("limit", mcplib.Description("Maximum entries to return."), mcplib.Min(1), mcplib.Max(200)),
		),
		handleSearchEntries(db),
	)
	srv.AddTool(
		mcplib.NewTool("list_nodes",
			mcplib.WithDescription("List registered Blackbox nodes."),
			mcplib.WithString("status", mcplib.Description("Filter by node status."), mcplib.Enum("online", "offline")),
		),
		handleListNodes(db),
	)
	return srv
}
