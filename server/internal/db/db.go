package db

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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

const legacyEntriesTimestampIndex = "idx_entries_timestamp"

func Init(path string) (*gorm.DB, error) {
	dsn := path
	if path == ":memory:" {
		dsn = fmt.Sprintf("file:blackbox-%d-%d?mode=memory&cache=shared", time.Now().UnixNano(), memoryDBCounter.Add(1))
	} else if err := ensureWritablePath(path); err != nil {
		return nil, err
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
		preservedAliases = normalizePreservedAliases(preservedAliases)
	}
	if err := database.AutoMigrate(
		&models.SetupState{},
		&models.User{},
		&models.InviteCode{},
		&models.OIDCState{},
		&models.OIDCProviderConfig{},
		&models.AppSetting{},
		&types.Entry{},
		&models.Node{},
		&models.EntryNote{},
		&models.ServiceAlias{},
		&models.Incident{},
		&models.IncidentEntry{},
		&models.SystemdUnitConfig{},
	); err != nil {
		return nil, err
	}
	if err := ensureEntryIndexes(database); err != nil {
		return nil, err
	}
	if err := database.Exec("DELETE FROM service_aliases WHERE TRIM(canonical) = '' OR canonical IS NULL OR TRIM(alias) = '' OR alias IS NULL").Error; err != nil {
		return nil, err
	}
	if len(preservedAliases) > 0 {
		if err := database.Exec("DELETE FROM service_aliases").Error; err != nil {
			return nil, err
		}
		if err := database.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "alias"}},
			DoUpdates: clause.AssignmentColumns([]string{"canonical"}),
		}).Create(&preservedAliases).Error; err != nil {
			return nil, err
		}
	}
	go sweepExpiredOIDCStates(database)
	return database, nil
}

func normalizePreservedAliases(aliases []models.ServiceAlias) []models.ServiceAlias {
	normalized := make([]models.ServiceAlias, 0, len(aliases))
	indexByAlias := make(map[string]int, len(aliases))

	for _, alias := range aliases {
		trimmedCanonical := strings.TrimSpace(alias.Canonical)
		trimmedAlias := strings.TrimSpace(alias.Alias)
		if trimmedCanonical == "" || trimmedAlias == "" {
			continue
		}

		if index, ok := indexByAlias[trimmedAlias]; ok {
			normalized[index].Canonical = trimmedCanonical
			continue
		}

		indexByAlias[trimmedAlias] = len(normalized)
		normalized = append(normalized, models.ServiceAlias{
			Canonical: trimmedCanonical,
			Alias:     trimmedAlias,
		})
	}

	return normalized
}

func ensureEntryIndexes(database *gorm.DB) error {
	if !database.Migrator().HasIndex(&types.Entry{}, "idx_entries_timestamp_id") {
		if err := database.Migrator().CreateIndex(&types.Entry{}, "idx_entries_timestamp_id"); err != nil {
			return err
		}
	}
	if database.Migrator().HasIndex(&types.Entry{}, legacyEntriesTimestampIndex) {
		if err := database.Migrator().DropIndex(&types.Entry{}, legacyEntriesTimestampIndex); err != nil {
			return err
		}
	}
	return nil
}

func ensureWritablePath(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("database directory %s is not writable by uid=%d gid=%d: %w", dir, os.Getuid(), os.Getgid(), err)
	}

	probe, err := os.CreateTemp(dir, ".blackbox-write-test-*")
	if err != nil {
		return fmt.Errorf("database directory %s is not writable by uid=%d gid=%d: %w", dir, os.Getuid(), os.Getgid(), err)
	}
	probeName := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probeName)
		return fmt.Errorf("database directory %s is not writable by uid=%d gid=%d: %w", dir, os.Getuid(), os.Getgid(), err)
	}
	if err := os.Remove(probeName); err != nil {
		return fmt.Errorf("database directory %s is not writable by uid=%d gid=%d: %w", dir, os.Getuid(), os.Getgid(), err)
	}

	if _, err := os.Stat(path); err == nil {
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("database file %s is not writable by uid=%d gid=%d: %w", path, os.Getuid(), os.Getgid(), err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("database file %s is not writable by uid=%d gid=%d: %w", path, os.Getuid(), os.Getgid(), err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("database file %s could not be checked by uid=%d gid=%d: %w", path, os.Getuid(), os.Getgid(), err)
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("database file %s could not be created by uid=%d gid=%d: %w", path, os.Getuid(), os.Getgid(), err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("database file %s could not be prepared by uid=%d gid=%d: %w", path, os.Getuid(), os.Getgid(), err)
	}

	return nil
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
