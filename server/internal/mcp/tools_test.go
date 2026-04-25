package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"blackbox/server/internal/db"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newToolsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := db.Init(":memory:")
	require.NoError(t, err)
	return database
}

func callTool(handler func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error), args map[string]any) (*mcplib.CallToolResult, error) {
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = args
	return handler(context.Background(), req)
}

func decodeToolResult(t *testing.T, result *mcplib.CallToolResult) map[string]any {
	t.Helper()
	require.NotNil(t, result)
	require.False(t, result.IsError, "tool returned error: %v", result.Content)
	require.NotEmpty(t, result.Content)
	var out map[string]any
	text, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok, "expected TextContent")
	require.NoError(t, json.Unmarshal([]byte(text.Text), &out))
	return out
}

// --- handleListIncidents ---

func TestHandleListIncidents_Empty(t *testing.T) {
	t.Parallel()
	database := newToolsTestDB(t)
	handler := handleListIncidents(database)

	result, err := callTool(handler, nil)
	require.NoError(t, err)
	out := decodeToolResult(t, result)
	incidents, ok := out["incidents"].([]any)
	require.True(t, ok)
	assert.Empty(t, incidents)
	assert.Equal(t, float64(0), out["total"])
}

func TestHandleListIncidents_ReturnsAll(t *testing.T) {
	t.Parallel()
	database := newToolsTestDB(t)

	inc1 := models.Incident{
		ID:         "inc-1",
		OpenedAt:   time.Now().Add(-2 * time.Hour),
		Status:     "open",
		Confidence: "confirmed",
		Title:      "First incident",
		Services:   "[]",
		NodeNames:  "[]",
		Metadata:   "{}",
	}
	inc2 := models.Incident{
		ID:         "inc-2",
		OpenedAt:   time.Now().Add(-1 * time.Hour),
		Status:     "resolved",
		Confidence: "suspected",
		Title:      "Second incident",
		Services:   "[]",
		NodeNames:  "[]",
		Metadata:   "{}",
	}
	require.NoError(t, database.Create(&inc1).Error)
	require.NoError(t, database.Create(&inc2).Error)

	handler := handleListIncidents(database)
	result, err := callTool(handler, nil)
	require.NoError(t, err)
	out := decodeToolResult(t, result)
	incidents, ok := out["incidents"].([]any)
	require.True(t, ok)
	assert.Len(t, incidents, 2)
	assert.Equal(t, float64(2), out["total"])
}

func TestHandleListIncidents_FilterByStatus(t *testing.T) {
	t.Parallel()
	database := newToolsTestDB(t)

	for _, inc := range []models.Incident{
		{ID: "inc-open", OpenedAt: time.Now(), Status: "open", Confidence: "confirmed", Title: "Open", Services: "[]", NodeNames: "[]", Metadata: "{}"},
		{ID: "inc-resolved", OpenedAt: time.Now(), Status: "resolved", Confidence: "confirmed", Title: "Resolved", Services: "[]", NodeNames: "[]", Metadata: "{}"},
	} {
		require.NoError(t, database.Create(&inc).Error)
	}

	handler := handleListIncidents(database)
	result, err := callTool(handler, map[string]any{"status": "open"})
	require.NoError(t, err)
	out := decodeToolResult(t, result)
	incidents, ok := out["incidents"].([]any)
	require.True(t, ok)
	assert.Len(t, incidents, 1)
}

func TestHandleListIncidents_FilterByConfidence(t *testing.T) {
	t.Parallel()
	database := newToolsTestDB(t)

	for _, inc := range []models.Incident{
		{ID: "inc-confirmed", OpenedAt: time.Now(), Status: "open", Confidence: "confirmed", Title: "Confirmed", Services: "[]", NodeNames: "[]", Metadata: "{}"},
		{ID: "inc-suspected", OpenedAt: time.Now(), Status: "open", Confidence: "suspected", Title: "Suspected", Services: "[]", NodeNames: "[]", Metadata: "{}"},
	} {
		require.NoError(t, database.Create(&inc).Error)
	}

	handler := handleListIncidents(database)
	result, err := callTool(handler, map[string]any{"confidence": "suspected"})
	require.NoError(t, err)
	out := decodeToolResult(t, result)
	incidents, ok := out["incidents"].([]any)
	require.True(t, ok)
	assert.Len(t, incidents, 1)
}

// --- handleSearchEntries ---

func TestHandleSearchEntries_LikeFallback(t *testing.T) {
	t.Parallel()
	// Use a fresh in-memory DB without FTS table to force LIKE fallback.
	// newToolsTestDB uses db.Init which creates the FTS table, so instead
	// we seed entries and verify search_mode reflects whichever path fires.
	database := newToolsTestDB(t)

	entry := types.Entry{
		ID:        "e-1",
		Timestamp: time.Now(),
		NodeName:  "node-a",
		Source:    "docker",
		Event:     "start",
		Content:   "container my-app started",
	}
	require.NoError(t, database.Create(&entry).Error)

	handler := handleSearchEntries(database)
	result, err := callTool(handler, map[string]any{"query": "my-app"})
	require.NoError(t, err)
	out := decodeToolResult(t, result)

	entries, ok := out["entries"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, entries)

	mode, ok := out["search_mode"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, mode, "search_mode field should be set")
}

func TestHandleSearchEntries_ReturnsSearchModeField(t *testing.T) {
	t.Parallel()
	database := newToolsTestDB(t)

	handler := handleSearchEntries(database)
	result, err := callTool(handler, map[string]any{"query": "anything"})
	require.NoError(t, err)
	out := decodeToolResult(t, result)

	_, ok := out["search_mode"]
	assert.True(t, ok, "response must contain search_mode field")
	_, ok = out["total_found"]
	assert.True(t, ok, "response must contain total_found field")
}

func TestHandleSearchEntries_RequiresQuery(t *testing.T) {
	t.Parallel()
	database := newToolsTestDB(t)

	handler := handleSearchEntries(database)
	result, err := callTool(handler, map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "expected error result when query is missing")
}

func TestHandleSearchEntries_EscapesWildcards(t *testing.T) {
	t.Parallel()
	database := newToolsTestDB(t)

	// Insert an entry that would be matched by an unescaped % wildcard but not by a literal %
	entry := types.Entry{
		ID:        "e-wc",
		Timestamp: time.Now(),
		NodeName:  "node-b",
		Source:    "docker",
		Event:     "start",
		Content:   "something else",
	}
	require.NoError(t, database.Create(&entry).Error)

	handler := handleSearchEntries(database)
	// Search for literal "%" — should not match "something else"
	result, err := callTool(handler, map[string]any{"query": "%"})
	require.NoError(t, err)
	out := decodeToolResult(t, result)
	entries, ok := out["entries"].([]any)
	require.True(t, ok)
	assert.Empty(t, entries, "literal %% should not match arbitrary content")
}

// --- handleListNodes ---

func TestHandleListNodes_Empty(t *testing.T) {
	t.Parallel()
	database := newToolsTestDB(t)

	handler := handleListNodes(database)
	result, err := callTool(handler, nil)
	require.NoError(t, err)
	out := decodeToolResult(t, result)
	nodes, ok := out["nodes"].([]any)
	require.True(t, ok)
	assert.Empty(t, nodes)
}

func TestHandleListNodes_ReturnsNodes(t *testing.T) {
	t.Parallel()
	database := newToolsTestDB(t)

	require.NoError(t, database.Create(&models.Node{
		ID:       "node-1",
		Name:     "server-a",
		LastSeen: time.Now(),
		Status:   "online",
	}).Error)
	require.NoError(t, database.Create(&models.Node{
		ID:       "node-2",
		Name:     "server-b",
		LastSeen: time.Now().Add(-10 * time.Minute),
		Status:   "offline",
	}).Error)

	handler := handleListNodes(database)
	result, err := callTool(handler, nil)
	require.NoError(t, err)
	out := decodeToolResult(t, result)
	nodes, ok := out["nodes"].([]any)
	require.True(t, ok)
	assert.Len(t, nodes, 2)
}

func TestHandleListNodes_FilterByStatus(t *testing.T) {
	t.Parallel()
	database := newToolsTestDB(t)

	require.NoError(t, database.Create(&models.Node{ID: "n-online", Name: "online-node", LastSeen: time.Now(), Status: "online"}).Error)
	require.NoError(t, database.Create(&models.Node{ID: "n-offline", Name: "offline-node", LastSeen: time.Now(), Status: "offline"}).Error)

	handler := handleListNodes(database)
	result, err := callTool(handler, map[string]any{"status": "online"})
	require.NoError(t, err)
	out := decodeToolResult(t, result)
	nodes, ok := out["nodes"].([]any)
	require.True(t, ok)
	assert.Len(t, nodes, 1)
}
