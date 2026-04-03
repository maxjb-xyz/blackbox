package services

import (
	"strings"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

func NormalizeService(db *gorm.DB, name string) string {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return ""
	}

	var alias models.ServiceAlias
	if err := db.Where("alias = ?", normalized).First(&alias).Error; err == nil {
		return alias.Canonical
	}

	return normalized
}
