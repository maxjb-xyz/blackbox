package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
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
	if err := database.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)

	if path != ":memory:" {
		if err := database.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
			return nil, fmt.Errorf("set WAL mode: %w", err)
		}
		if err := database.Exec("PRAGMA busy_timeout=5000").Error; err != nil {
			return nil, fmt.Errorf("set busy timeout: %w", err)
		}
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
		&models.Incident{},
		&models.IncidentEntry{},
		&models.SystemdUnitConfig{},
		&models.NotificationDest{},
		&models.ExcludedTarget{},
		&models.AuditLog{},
		&models.WebhookDelivery{},
		&models.DataSourceInstance{},
	); err != nil {
		return nil, err
	}
	if err := ensureEntryIndexes(database); err != nil {
		return nil, err
	}
	if err := ensureExcludedTargetIndexes(database); err != nil {
		return nil, err
	}
	if err := ensureDataSourceConstraints(database); err != nil {
		return nil, err
	}
	if err := ensureDataSourceCleanupTriggers(database); err != nil {
		return nil, err
	}
	if err := EnsureEntriesFTS(database); err != nil {
		return nil, err
	}
	return database, nil
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

func ensureExcludedTargetIndexes(database *gorm.DB) error {
	return database.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_excluded_targets_type_lower_name
ON excluded_targets(type, lower(name));
`).Error
}

func ensureDataSourceConstraints(database *gorm.DB) error {
	if err := database.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_data_source_singleton_agent
ON data_source_instances(type, node_id)
WHERE scope = 'agent' AND type IN ('docker', 'systemd', 'filewatcher', 'proxmox')
`).Error; err != nil {
		return err
	}
	return database.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_data_source_singleton_server
ON data_source_instances(type)
WHERE scope = 'server' AND type IN ('webhook_uptime_kuma', 'webhook_watchtower')
`).Error
}

func ensureDataSourceCleanupTriggers(database *gorm.DB) error {
	return database.Exec(`
CREATE TRIGGER IF NOT EXISTS trg_data_source_instances_delete_node
AFTER DELETE ON nodes
FOR EACH ROW
BEGIN
  DELETE FROM data_source_instances
  WHERE scope = 'agent' AND node_id = OLD.name;
END
`).Error
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

func deleteExpiredOIDCStates(database *gorm.DB) {
	result := database.Delete(&models.OIDCState{}, "expires_at < ?", time.Now())
	if result.Error != nil {
		log.Printf("oidc state sweep error: %v", result.Error)
	}
}

func sweepExpiredOIDCStates(ctx context.Context, database *gorm.DB) {
	deleteExpiredOIDCStates(database)

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleteExpiredOIDCStates(database)
		}
	}
}

func StartOIDCStateSweeper(ctx context.Context, database *gorm.DB) {
	go sweepExpiredOIDCStates(ctx, database)
}
