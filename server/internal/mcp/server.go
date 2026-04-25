package mcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

// MCPManager manages the lifecycle of the embedded MCP HTTP/SSE server.
type MCPManager struct {
	mu      sync.Mutex
	running *http.Server
	cancel  context.CancelFunc
	port    int // currently bound port, 0 if not running
	db      *gorm.DB
}

func NewMCPManager(db *gorm.DB) *MCPManager {
	return &MCPManager{db: db}
}

func (m *MCPManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running != nil
}

func (m *MCPManager) ApplySettings(enabled bool, port int, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !enabled {
		_ = m.stopLocked()
		m.port = 0
		return nil
	}
	if token == "" {
		return errors.New("mcp auth token is required")
	}
	if port < 1024 || port > 65535 {
		return errors.New("mcp port must be between 1024 and 65535")
	}

	addr := fmt.Sprintf(":%d", port)

	var ln net.Listener
	if m.running != nil && m.port == port {
		// Same port: stop first, then retry bind briefly while the OS releases the socket.
		_ = m.stopLocked()
		var err error
		ln, err = listenWithRetry(addr, 25, 10*time.Millisecond)
		if err != nil {
			m.port = 0
			return fmt.Errorf("mcp: bind %s: %w", addr, err)
		}
	} else {
		// Different port (or not running): bind first to prove it works before stopping old server.
		var err error
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("mcp: bind %s: %w", addr, err)
		}
		_ = m.stopLocked()
	}

	ctx, cancel := context.WithCancel(context.Background())
	server := buildServer(m.db)
	sseServer := mcpserver.NewSSEServer(server)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           BearerTokenMiddleware(token, sseServer),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	m.running = httpServer
	m.cancel = cancel
	m.port = port
	go func() {
		if serveErr := httpServer.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("mcp: server error: %v", serveErr)
			m.mu.Lock()
			if m.running == httpServer {
				m.running = nil
				m.cancel = nil
				m.port = 0
			}
			m.mu.Unlock()
		}
	}()
	log.Printf("mcp: server listening on %s", httpServer.Addr)
	return nil
}

func (m *MCPManager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLockedWithContext(ctx)
}

func (m *MCPManager) stopLocked() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.stopLockedWithContext(ctx)
}

func (m *MCPManager) stopLockedWithContext(ctx context.Context) error {
	if m.running == nil {
		return nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	err := m.running.Shutdown(ctx)
	m.running = nil
	m.cancel = nil
	m.port = 0
	log.Printf("mcp: server stopped")
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func listenWithRetry(addr string, attempts int, delay time.Duration) (net.Listener, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
		lastErr = err
		if i < attempts-1 {
			time.Sleep(delay)
		}
	}
	return nil, lastErr
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
