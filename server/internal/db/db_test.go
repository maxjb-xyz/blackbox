package db_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"blackbox/server/internal/db"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type sqliteIndexInfoRow struct {
	Seqno int    `gorm:"column:seqno"`
	Name  string `gorm:"column:name"`
}

func closeDBOnCleanup(t *testing.T, database *gorm.DB) {
	t.Helper()
	t.Cleanup(func() {
		sqlDB, err := database.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
}

func TestInit_CreatesTablesAndMigrates(t *testing.T) {
	tmp, err := os.CreateTemp("", "blackbox-test-*.db")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())
	t.Cleanup(func() {
		require.NoError(t, os.Remove(tmp.Name()))
	})

	database, err := db.Init(tmp.Name())
	require.NoError(t, err)
	assert.NotNil(t, database)
	closeDBOnCleanup(t, database)

	user := models.User{ID: "01TESTUSER", Username: "admin", IsAdmin: true}
	assert.NoError(t, database.Create(&user).Error)

	var found models.User
	assert.NoError(t, database.First(&found, "username = ?", "admin").Error)
	assert.Equal(t, "admin", found.Username)
	assert.True(t, found.IsAdmin)

	entry := types.Entry{ID: "01TESTENTRY", NodeName: "node1", Source: "manual", Event: "test"}
	assert.NoError(t, database.Create(&entry).Error)
}

func TestIncidentTablesExist(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)
	closeDBOnCleanup(t, database)

	assert.True(t, database.Migrator().HasTable(&models.Incident{}))
	assert.True(t, database.Migrator().HasTable(&models.IncidentEntry{}))
}

func TestInit_CreatesCompositeEntryCursorIndex(t *testing.T) {
	tmp, err := os.CreateTemp("", "blackbox-test-*.db")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())
	t.Cleanup(func() {
		require.NoError(t, os.Remove(tmp.Name()))
	})

	database, err := db.Init(tmp.Name())
	require.NoError(t, err)
	closeDBOnCleanup(t, database)

	require.True(t, database.Migrator().HasIndex(&types.Entry{}, "idx_entries_timestamp_id"))
	require.False(t, database.Migrator().HasIndex(&types.Entry{}, "idx_entries_timestamp"))

	var columns []sqliteIndexInfoRow
	require.NoError(t, database.Raw(`PRAGMA index_info('idx_entries_timestamp_id')`).Scan(&columns).Error)
	require.Len(t, columns, 2)
	assert.Equal(t, "timestamp", columns[0].Name)
	assert.Equal(t, "id", columns[1].Name)
}

func TestInit_EnforcesSingletonDataSourceUniqueness(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)
	closeDBOnCleanup(t, database)

	nodeName := "node-1"
	require.NoError(t, database.Create(&models.Node{ID: "n1", Name: nodeName, Capabilities: "[]"}).Error)
	require.NoError(t, database.Create(&models.DataSourceInstance{
		ID: "sys-1", Type: "systemd", Scope: "agent", NodeID: &nodeName, Name: "Systemd", Config: "{}",
	}).Error)
	err = database.Create(&models.DataSourceInstance{
		ID: "sys-2", Type: "systemd", Scope: "agent", NodeID: &nodeName, Name: "Systemd 2", Config: "{}",
	}).Error
	require.Error(t, err)

	require.NoError(t, database.Create(&models.DataSourceInstance{
		ID: "wh-1", Type: "webhook_uptime_kuma", Scope: "server", Name: "UK", Config: "{}",
	}).Error)
	err = database.Create(&models.DataSourceInstance{
		ID: "wh-2", Type: "webhook_uptime_kuma", Scope: "server", Name: "UK 2", Config: "{}",
	}).Error
	require.Error(t, err)
}

func TestInit_CascadesAgentScopedDataSourcesWhenNodeDeleted(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)
	closeDBOnCleanup(t, database)

	nodeName := "node-1"
	require.NoError(t, database.Create(&models.Node{ID: "n1", Name: nodeName, Capabilities: "[]"}).Error)
	require.NoError(t, database.Create(&models.DataSourceInstance{
		ID: "sys-1", Type: "systemd", Scope: "agent", NodeID: &nodeName, Name: "Systemd", Config: "{}",
	}).Error)

	require.NoError(t, database.Delete(&models.Node{}, "name = ?", nodeName).Error)

	var count int64
	require.NoError(t, database.Model(&models.DataSourceInstance{}).Where("id = ?", "sys-1").Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestInit_DropsLegacyTimestampOnlyEntryIndex(t *testing.T) {
	tmp, err := os.CreateTemp("", "blackbox-test-*.db")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())
	t.Cleanup(func() {
		require.NoError(t, os.Remove(tmp.Name()))
	})

	legacyDB, err := gorm.Open(sqlite.Open(tmp.Name()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, legacyDB.Exec(`
		CREATE TABLE entries (
			id TEXT PRIMARY KEY,
			timestamp DATETIME,
			node_name TEXT,
			source TEXT,
			service TEXT,
			event TEXT,
			content TEXT,
			metadata TEXT,
			correlated_id TEXT
		)
	`).Error)
	require.NoError(t, legacyDB.Exec(`CREATE INDEX idx_entries_timestamp ON entries(timestamp)`).Error)
	require.True(t, legacyDB.Migrator().HasIndex(&types.Entry{}, "idx_entries_timestamp"))

	sqlDB, err := legacyDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	database, err := db.Init(tmp.Name())
	require.NoError(t, err)
	closeDBOnCleanup(t, database)

	require.True(t, database.Migrator().HasIndex(&types.Entry{}, "idx_entries_timestamp_id"))
	require.False(t, database.Migrator().HasIndex(&types.Entry{}, "idx_entries_timestamp"))
}

func TestInit_MigratesInviteCodeOIDCStateAndOIDCConfig(t *testing.T) {
	tmp, err := os.CreateTemp("", "blackbox-test-*.db")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())
	t.Cleanup(func() {
		require.NoError(t, os.Remove(tmp.Name()))
	})

	database, err := db.Init(tmp.Name())
	require.NoError(t, err)
	closeDBOnCleanup(t, database)

	invite := models.InviteCode{
		ID:        "01INVITEID0000000",
		Code:      "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		CreatedBy: "01ADMINID0000000",
		ExpiresAt: time.Now().Add(72 * time.Hour),
		CreatedAt: time.Now(),
	}
	assert.NoError(t, database.Create(&invite).Error)

	state := models.OIDCState{
		ID:         "01STATEID00000000",
		State:      "randomstate123456789012345678901234567890123456789012345678901234",
		Nonce:      "randomnonce123456789012345678901234567890123456789012345678901234",
		ProviderID: "01PROVIDERID000000",
		InviteCode: "invite-code-123",
		ExpiresAt:  time.Now().Add(10 * time.Minute),
		CreatedAt:  time.Now(),
	}
	assert.NoError(t, database.Create(&state).Error)

	provider := models.OIDCProviderConfig{
		ID:           "01PROVIDERID000000",
		Name:         "SSO",
		Issuer:       "https://issuer.example.com",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://app.example.com/callback",
		Enabled:      models.BoolPtr(true),
	}
	assert.NoError(t, database.Create(&provider).Error)

	setting := models.AppSetting{
		Key:       "oidc_policy",
		Value:     "open",
		UpdatedAt: time.Now(),
	}
	assert.NoError(t, database.Create(&setting).Error)
}

func TestInit_CreatesMissingDatabasePath(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "nested", "blackbox.db")

	database, err := db.Init(dbPath)
	require.NoError(t, err)
	assert.NotNil(t, database)
	closeDBOnCleanup(t, database)

	info, err := os.Stat(dbPath)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestInit_ReturnsHelpfulErrorForReadOnlyDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directory mode bits do not reliably simulate a read-only directory for this test")
	}
	if os.Getuid() == 0 {
		t.Skip("root can bypass directory permissions")
	}

	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	require.NoError(t, os.Mkdir(locked, 0o555))

	_, err := db.Init(filepath.Join(locked, "blackbox.db"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database directory")
	assert.Contains(t, err.Error(), "uid=")
	assert.Contains(t, err.Error(), "gid=")
}

func TestStartOIDCStateSweeper_DeletesExpiredStatesImmediately(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)
	closeDBOnCleanup(t, database)

	expired := models.OIDCState{
		ID:         "01STATEEXPIRED0000",
		State:      "expiredstate1234567890123456789012345678901234567890123456789012",
		Nonce:      "expirednonce1234567890123456789012345678901234567890123456789012",
		ProviderID: "01PROVIDEREXPIRED0",
		ExpiresAt:  time.Now().Add(-time.Minute),
		CreatedAt:  time.Now().Add(-2 * time.Minute),
	}
	fresh := models.OIDCState{
		ID:         "01STATEFRESH000000",
		State:      "freshstate123456789012345678901234567890123456789012345678901234",
		Nonce:      "freshnonce123456789012345678901234567890123456789012345678901234",
		ProviderID: "01PROVIDERFRESH000",
		ExpiresAt:  time.Now().Add(time.Hour),
		CreatedAt:  time.Now(),
	}
	require.NoError(t, database.Create(&expired).Error)
	require.NoError(t, database.Create(&fresh).Error)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db.StartOIDCStateSweeper(ctx, database)

	require.Eventually(t, func() bool {
		var expiredCount int64
		if err := database.Model(&models.OIDCState{}).Where("id = ?", expired.ID).Count(&expiredCount).Error; err != nil {
			return false
		}
		return expiredCount == 0
	}, time.Second, 10*time.Millisecond)

	var freshCount int64
	require.NoError(t, database.Model(&models.OIDCState{}).Where("id = ?", fresh.ID).Count(&freshCount).Error)
	assert.Equal(t, int64(1), freshCount)
}

func TestInit_FileDB_WALModeEnabled(t *testing.T) {
	tmp, err := os.CreateTemp("", "blackbox-wal-test-*.db")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())
	t.Cleanup(func() { require.NoError(t, os.Remove(tmp.Name())) })

	database, err := db.Init(tmp.Name())
	require.NoError(t, err)
	closeDBOnCleanup(t, database)

	var mode string
	require.NoError(t, database.Raw("PRAGMA journal_mode").Scan(&mode).Error)
	assert.Equal(t, "wal", mode)
}

func TestInit_FileDB_BusyTimeoutSet(t *testing.T) {
	tmp, err := os.CreateTemp("", "blackbox-busy-test-*.db")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())
	t.Cleanup(func() { require.NoError(t, os.Remove(tmp.Name())) })

	database, err := db.Init(tmp.Name())
	require.NoError(t, err)
	closeDBOnCleanup(t, database)

	var timeout int
	require.NoError(t, database.Raw("PRAGMA busy_timeout").Scan(&timeout).Error)
	assert.Equal(t, 5000, timeout)
}

func TestInit_FileDB_SingleOpenConnection(t *testing.T) {
	tmp, err := os.CreateTemp("", "blackbox-conn-test-*.db")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())
	t.Cleanup(func() { require.NoError(t, os.Remove(tmp.Name())) })

	database, err := db.Init(tmp.Name())
	require.NoError(t, err)
	closeDBOnCleanup(t, database)

	sqlDB, err := database.DB()
	require.NoError(t, err)
	stats := sqlDB.Stats()
	assert.Equal(t, 1, stats.MaxOpenConnections)
}
