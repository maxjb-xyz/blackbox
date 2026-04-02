package db

import (
	"log"
	"time"

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
	if err := database.AutoMigrate(
		&models.User{},
		&models.InviteCode{},
		&models.OIDCState{},
		&types.Entry{},
	); err != nil {
		return nil, err
	}
	go sweepExpiredOIDCStates(database)
	return database, nil
}

func sweepExpiredOIDCStates(database *gorm.DB) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		result := database.Delete(&models.OIDCState{}, "expires_at < ?", time.Now())
		if result.Error != nil {
			log.Printf("oidc state sweep error: %v", result.Error)
		}
	}
}
