package db

import (
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Init(path string) (*gorm.DB, error) {
	database, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(&models.User{}, &types.Entry{}); err != nil {
		return nil, err
	}
	return database, nil
}
