package db

import (
	"fmt"
	"log"
	"time"

	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Init(path string) (*gorm.DB, error) {
	dsn := path
	if path == ":memory:" {
		dsn = fmt.Sprintf("file:blackbox-%d?mode=memory&cache=shared", time.Now().UnixNano())
	}
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		sqlDB, err := database.DB()
		if err != nil {
			return nil, err
		}
		sqlDB.SetMaxOpenConns(1)
	}
	if err := database.AutoMigrate(
		&models.User{},
		&models.InviteCode{},
		&models.OIDCState{},
		&types.Entry{},
		&models.Node{},
		&models.EntryNote{},
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
