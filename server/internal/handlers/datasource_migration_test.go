package handlers_test

import (
	"encoding/json"
	"testing"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.AppSetting{},
		&models.SystemdUnitConfig{},
		&models.Node{},
		&models.DataSourceInstance{},
	))
	return db
}

func TestMigrateDataSources_SystemdRows(t *testing.T) {
	db := newMigrationTestDB(t)

	units, err := json.Marshal([]string{"nginx.service", "caddy.service"})
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.SystemdUnitConfig{
		NodeName: "homelab-01",
		Units:    string(units),
	}).Error)

	require.NoError(t, handlers.MigrateDataSources(db, ""))

	var instances []models.DataSourceInstance
	require.NoError(t, db.Where("type = ?", "systemd").Find(&instances).Error)
	require.Len(t, instances, 1)
	require.NotNil(t, instances[0].NodeID)
	require.Equal(t, "homelab-01", *instances[0].NodeID)
	require.Equal(t, "agent", instances[0].Scope)

	var cfg struct {
		Units []string `json:"units"`
	}
	require.NoError(t, json.Unmarshal([]byte(instances[0].Config), &cfg))
	require.Equal(t, []string{"nginx.service", "caddy.service"}, cfg.Units)
}

func TestMigrateDataSources_FileWatcherPerNode(t *testing.T) {
	db := newMigrationTestDB(t)

	require.NoError(t, db.Create(&models.Node{ID: "n1", Name: "homelab-01", Capabilities: `["filewatcher"]`}).Error)
	require.NoError(t, db.Create(&models.AppSetting{Key: "file_watcher_redact_secrets", Value: "true"}).Error)

	require.NoError(t, handlers.MigrateDataSources(db, ""))

	var instances []models.DataSourceInstance
	require.NoError(t, db.Where("type = ?", "filewatcher").Find(&instances).Error)
	require.Len(t, instances, 1)
	require.NotNil(t, instances[0].NodeID)
	require.Equal(t, "homelab-01", *instances[0].NodeID)
	require.True(t, instances[0].Enabled)

	var cfg struct {
		RedactSecrets bool `json:"redact_secrets"`
	}
	require.NoError(t, json.Unmarshal([]byte(instances[0].Config), &cfg))
	require.True(t, cfg.RedactSecrets)
}

func TestMigrateDataSources_FileWatcherDisabledWithoutCapability(t *testing.T) {
	db := newMigrationTestDB(t)

	require.NoError(t, db.Create(&models.Node{ID: "n1", Name: "homelab-01", Capabilities: "[]"}).Error)
	require.NoError(t, handlers.MigrateDataSources(db, ""))

	var count int64
	require.NoError(t, db.Model(&models.DataSourceInstance{}).Where("type = ?", "filewatcher").Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestMigrateDataSources_WebhookInstances(t *testing.T) {
	db := newMigrationTestDB(t)

	require.NoError(t, handlers.MigrateDataSources(db, "mysecret"))

	var uptime, watchtower models.DataSourceInstance
	require.NoError(t, db.Where("type = ?", "webhook_uptime_kuma").First(&uptime).Error)
	require.NoError(t, db.Where("type = ?", "webhook_watchtower").First(&watchtower).Error)

	var cfgUptime struct {
		Secret string `json:"secret"`
	}
	var cfgWatchtower struct {
		Secret string `json:"secret"`
	}
	require.NoError(t, json.Unmarshal([]byte(uptime.Config), &cfgUptime))
	require.Equal(t, "mysecret", cfgUptime.Secret)
	require.NoError(t, json.Unmarshal([]byte(watchtower.Config), &cfgWatchtower))
	require.Equal(t, "mysecret", cfgWatchtower.Secret)
}

func TestMigrateDataSources_Idempotent(t *testing.T) {
	db := newMigrationTestDB(t)
	require.NoError(t, db.Create(&models.Node{ID: "n1", Name: "homelab-01", Capabilities: `["filewatcher"]`}).Error)
	units, err := json.Marshal([]string{"nginx.service"})
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.SystemdUnitConfig{
		NodeName: "homelab-01",
		Units:    string(units),
	}).Error)
	require.NoError(t, handlers.MigrateDataSources(db, "s"))
	require.NoError(t, handlers.MigrateDataSources(db, "s"))

	for _, sourceType := range []string{"webhook_uptime_kuma", "webhook_watchtower", "systemd", "filewatcher"} {
		var count int64
		require.NoError(t, db.Model(&models.DataSourceInstance{}).Where("type = ?", sourceType).Count(&count).Error)
		require.Equal(t, int64(1), count, "unexpected count for %s", sourceType)
	}
}
