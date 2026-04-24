package handlers

import (
	"encoding/json"
	"strings"

	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"gorm.io/gorm"
)

func isExcluded(database *gorm.DB, entry types.Entry) bool {
	if database == nil || entry.Source != "docker" {
		return false
	}
	service := strings.ToLower(strings.TrimSpace(entry.Service))
	stack := strings.ToLower(strings.TrimSpace(extractComposeProject(entry.Metadata)))
	if service == "" && stack == "" {
		return false
	}

	var count int64
	tx := database.Model(&models.ExcludedTarget{}).Where(
		"(type = ? AND lower(name) = ?) OR (type = ? AND lower(name) = ?)",
		"container", service,
		"stack", stack,
	).Count(&count)
	return tx.Error == nil && count > 0
}

func extractComposeProject(metadata string) string {
	if strings.TrimSpace(metadata) == "" {
		return ""
	}
	var value any
	if err := json.Unmarshal([]byte(metadata), &value); err != nil {
		return ""
	}
	for _, key := range []string{
		"com.docker.compose.project",
		"compose_project",
		"composeProject",
		"stack",
	} {
		if found := findStringValue(value, key); found != "" {
			return found
		}
	}
	return ""
}

func findStringValue(value any, key string) string {
	switch v := value.(type) {
	case map[string]any:
		for k, child := range v {
			if strings.EqualFold(k, key) {
				if s, ok := child.(string); ok {
					return strings.TrimSpace(s)
				}
			}
			if found := findStringValue(child, key); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range v {
			if found := findStringValue(child, key); found != "" {
				return found
			}
		}
	}
	return ""
}
