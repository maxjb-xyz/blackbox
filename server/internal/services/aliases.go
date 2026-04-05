package services

import (
	"errors"
	"strings"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

func normalizeServiceName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func NormalizeService(db *gorm.DB, name string) (string, error) {
	normalized := normalizeServiceName(name)
	if normalized == "" {
		return "", nil
	}

	var alias models.ServiceAlias
	err := db.Where("LOWER(alias) = ?", normalized).First(&alias).Error
	switch {
	case err == nil:
		return normalizeServiceName(alias.Canonical), nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return normalized, nil
	case err != nil:
		return "", err
	}

	return normalized, nil
}
