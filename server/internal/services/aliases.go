package services

import (
	"errors"
	"strings"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

func NormalizeService(db *gorm.DB, name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", nil
	}

	var alias models.ServiceAlias
	err := db.Where("alias = ?", normalized).First(&alias).Error
	switch {
	case err == nil:
		return alias.Canonical, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return normalized, nil
	case err != nil:
		return "", err
	}

	return normalized, nil
}
