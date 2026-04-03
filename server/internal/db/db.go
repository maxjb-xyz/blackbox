package db

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var memoryDBCounter atomic.Uint64

func Init(path string) (*gorm.DB, error) {
	dsn := path
	if path == ":memory:" {
		dsn = fmt.Sprintf("file:blackbox-%d-%d?mode=memory&cache=shared", time.Now().UnixNano(), memoryDBCounter.Add(1))
	}
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
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
	var preservedAliases []models.ServiceAlias
	if database.Migrator().HasTable(&models.ServiceAlias{}) {
		if err := database.Exec("DELETE FROM service_aliases WHERE TRIM(canonical) = '' OR canonical IS NULL OR TRIM(alias) = '' OR alias IS NULL").Error; err != nil {
			return nil, err
		}
		if err := database.Raw("SELECT TRIM(canonical) AS canonical, TRIM(alias) AS alias FROM service_aliases").Scan(&preservedAliases).Error; err != nil {
			return nil, err
		}
	}
	if err := database.AutoMigrate(
		&models.User{},
		&models.InviteCode{},
		&models.OIDCState{},
		&types.Entry{},
		&models.Node{},
		&models.EntryNote{},
		&models.ServiceAlias{},
	); err != nil {
		return nil, err
	}
	if err := database.Exec("DELETE FROM service_aliases WHERE TRIM(canonical) = '' OR canonical IS NULL OR TRIM(alias) = '' OR alias IS NULL").Error; err != nil {
		return nil, err
	}
	if len(preservedAliases) > 0 {
		if err := database.Clauses(clause.OnConflict{DoNothing: true}).Create(&preservedAliases).Error; err != nil {
			return nil, err
		}
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
