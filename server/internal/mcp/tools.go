package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"blackbox/shared/types"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"gorm.io/gorm"
)

func toolResult(v any) (*mcplib.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("marshal error: %v", err)), nil
	}
	return mcplib.NewToolResultText(string(b)), nil
}

func argString(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func argInt(args map[string]any, key string, defaultVal, max int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			i := int(n)
			if i > 0 && i <= max {
				return i
			}
			if i > max {
				return max
			}
		case int:
			if n > 0 && n <= max {
				return n
			}
			if n > max {
				return max
			}
		}
	}
	return defaultVal
}

type incidentSummary struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Status     string     `json:"status"`
	Confidence string     `json:"confidence"`
	OpenedAt   time.Time  `json:"opened_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
	Services   []string   `json:"services"`
	NodeNames  []string   `json:"node_names"`
	EntryCount int64      `json:"entry_count"`
}

type entryCountRow struct {
	IncidentID string `gorm:"column:incident_id"`
	Count      int64  `gorm:"column:count"`
}

func handleListIncidents(db *gorm.DB) func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.Params.Arguments
		status := argString(args, "status")
		confidence := argString(args, "confidence")
		limit := argInt(args, "limit", 20, 100)

		tx := db.WithContext(ctx).Model(&models.Incident{}).Order("opened_at DESC").Limit(limit)
		if status != "" {
			tx = tx.Where("status = ?", status)
		}
		if confidence != "" {
			tx = tx.Where("confidence = ?", confidence)
		}

		var incidents []models.Incident
		if err := tx.Find(&incidents).Error; err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("db error: %v", err)), nil
		}

		ids := make([]string, len(incidents))
		for i, inc := range incidents {
			ids[i] = inc.ID
		}
		countMap := make(map[string]int64, len(ids))
		if len(ids) > 0 {
			var rows []entryCountRow
			db.WithContext(ctx).Table("incident_entries").Select("incident_id, COUNT(*) as count").
				Where("incident_id IN ?", ids).Group("incident_id").Scan(&rows)
			for _, row := range rows {
				countMap[row.IncidentID] = row.Count
			}
		}

		summaries := make([]incidentSummary, 0, len(incidents))
		for _, inc := range incidents {
			summaries = append(summaries, incidentSummary{
				ID:         inc.ID,
				Title:      inc.Title,
				Status:     inc.Status,
				Confidence: inc.Confidence,
				OpenedAt:   inc.OpenedAt,
				ResolvedAt: inc.ResolvedAt,
				Services:   parseStringList(inc.Services),
				NodeNames:  parseStringList(inc.NodeNames),
				EntryCount: countMap[inc.ID],
			})
		}

		return toolResult(map[string]any{"incidents": summaries, "total": len(summaries)})
	}
}

type incidentDetailEntry struct {
	Link models.IncidentEntry `json:"link"`
	Data *types.Entry         `json:"entry"`
}

func handleGetIncident(db *gorm.DB) func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		id := argString(req.Params.Arguments, "id")
		if id == "" {
			return mcplib.NewToolResultError("id is required"), nil
		}

		var incident models.Incident
		if err := db.WithContext(ctx).First(&incident, "id = ?", id).Error; err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("incident %s not found", id)), nil
		}
		var links []models.IncidentEntry
		if err := db.WithContext(ctx).Where("incident_id = ?", id).Order("score DESC").Find(&links).Error; err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("db error fetching entries: %v", err)), nil
		}
		details := make([]incidentDetailEntry, 0, len(links))
		for _, link := range links {
			var entry types.Entry
			if err := db.WithContext(ctx).First(&entry, "id = ?", link.EntryID).Error; err == nil {
				details = append(details, incidentDetailEntry{Link: link, Data: &entry})
			}
		}
		return toolResult(map[string]any{"incident": incident, "entries": details})
	}
}

func handleListEntries(db *gorm.DB) func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.Params.Arguments
		nodeID := argString(args, "node_id")
		source := argString(args, "source")
		event := argString(args, "event")
		since := argString(args, "since")
		until := argString(args, "until")
		cursor := argString(args, "cursor")
		limit := argInt(args, "limit", 50, 200)

		tx := db.WithContext(ctx).Model(&types.Entry{}).Order("timestamp DESC").Order("id DESC").Limit(limit + 1)
		if cursor != "" {
			ts, curID, ok := strings.Cut(cursor, "|")
			if !ok {
				return mcplib.NewToolResultError("invalid cursor format"), nil
			}
			parsed, err := time.Parse(time.RFC3339Nano, ts)
			if err != nil {
				return mcplib.NewToolResultError("invalid cursor timestamp"), nil
			}
			tx = tx.Where("timestamp < ? OR (timestamp = ? AND id < ?)", parsed, parsed, curID)
		}
		if since != "" {
			t, err := time.Parse(time.RFC3339, since)
			if err != nil {
				return mcplib.NewToolResultError("invalid since timestamp (use RFC3339)"), nil
			}
			tx = tx.Where("timestamp >= ?", t)
		}
		if until != "" {
			t, err := time.Parse(time.RFC3339, until)
			if err != nil {
				return mcplib.NewToolResultError("invalid until timestamp (use RFC3339)"), nil
			}
			tx = tx.Where("timestamp <= ?", t)
		}
		if nodeID != "" {
			tx = tx.Where("node_name = ?", nodeID)
		}
		if source != "" {
			tx = tx.Where("source = ?", source)
		}
		if event != "" {
			tx = tx.Where("event = ?", event)
		}

		var entries []types.Entry
		if err := tx.Find(&entries).Error; err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("db error: %v", err)), nil
		}
		nextCursor := ""
		if len(entries) > limit {
			entries = entries[:limit]
			last := entries[len(entries)-1]
			nextCursor = last.Timestamp.UTC().Format(time.RFC3339Nano) + "|" + last.ID
		}
		if entries == nil {
			entries = []types.Entry{}
		}
		return toolResult(map[string]any{"entries": entries, "next_cursor": nextCursor})
	}
}

func handleSearchEntries(db *gorm.DB) func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.Params.Arguments
		query := argString(args, "query")
		if query == "" {
			return mcplib.NewToolResultError("query is required"), nil
		}
		limit := argInt(args, "limit", 50, 200)
		since := argString(args, "since")
		var sinceTime *time.Time
		if since != "" {
			t, err := time.Parse(time.RFC3339, since)
			if err != nil {
				return mcplib.NewToolResultError("invalid since timestamp (use RFC3339)"), nil
			}
			sinceTime = &t
		}

		entries, mode, err := searchFTS5(ctx, db, query, sinceTime, limit)
		if err != nil {
			entries, err = searchLike(ctx, db, query, sinceTime, limit)
			if err != nil {
				return mcplib.NewToolResultError(fmt.Sprintf("search error: %v", err)), nil
			}
			mode = "like"
		}
		if entries == nil {
			entries = []types.Entry{}
		}
		return toolResult(map[string]any{"entries": entries, "search_mode": mode, "total_found": len(entries)})
	}
}

func searchFTS5(ctx context.Context, db *gorm.DB, query string, since *time.Time, limit int) ([]types.Entry, string, error) {
	tx := db.WithContext(ctx).Table("entries").Select("entries.*").
		Joins("JOIN entries_fts ON entries.rowid = entries_fts.rowid").
		Where("entries_fts MATCH ?", query).Order("rank").Limit(limit)
	if since != nil {
		tx = tx.Where("entries.timestamp >= ?", since)
	}
	var entries []types.Entry
	if err := tx.Find(&entries).Error; err != nil {
		return nil, "", err
	}
	return entries, "fts5", nil
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

func searchLike(ctx context.Context, db *gorm.DB, query string, since *time.Time, limit int) ([]types.Entry, error) {
	likePattern := "%" + escapeLike(query) + "%"
	tx := db.WithContext(ctx).Model(&types.Entry{}).
		Where("content LIKE ? ESCAPE '\\' OR service LIKE ? ESCAPE '\\'", likePattern, likePattern).Order("timestamp DESC").Limit(limit)
	if since != nil {
		tx = tx.Where("timestamp >= ?", since)
	}
	var entries []types.Entry
	return entries, tx.Find(&entries).Error
}

func handleListNodes(db *gorm.DB) func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		status := argString(req.Params.Arguments, "status")
		tx := db.WithContext(ctx).Model(&models.Node{}).Order("last_seen DESC")
		if status != "" {
			tx = tx.Where("status = ?", status)
		}
		var nodes []models.Node
		if err := tx.Find(&nodes).Error; err != nil {
			return mcplib.NewToolResultError(fmt.Sprintf("db error: %v", err)), nil
		}
		if nodes == nil {
			nodes = []models.Node{}
		}
		return toolResult(map[string]any{"nodes": nodes})
	}
}

func parseStringList(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	if values == nil {
		return []string{}
	}
	return values
}
