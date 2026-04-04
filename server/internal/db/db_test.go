package db_test

import (
	"os"
	"path/filepath"
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

	user := models.User{ID: "01TESTUSER", Username: "admin", IsAdmin: true}
	assert.NoError(t, database.Create(&user).Error)

	var found models.User
	assert.NoError(t, database.First(&found, "username = ?", "admin").Error)
	assert.Equal(t, "admin", found.Username)
	assert.True(t, found.IsAdmin)

	entry := types.Entry{ID: "01TESTENTRY", NodeName: "node1", Source: "manual", Event: "test"}
	assert.NoError(t, database.Create(&entry).Error)
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

	require.True(t, database.Migrator().HasIndex(&types.Entry{}, "idx_entries_timestamp_id"))
	require.False(t, database.Migrator().HasIndex(&types.Entry{}, "idx_entries_timestamp"))

	var columns []sqliteIndexInfoRow
	require.NoError(t, database.Raw(`PRAGMA index_info('idx_entries_timestamp_id')`).Scan(&columns).Error)
	require.Len(t, columns, 2)
	assert.Equal(t, "timestamp", columns[0].Name)
	assert.Equal(t, "id", columns[1].Name)
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

	invite := models.InviteCode{
		ID:        "01INVITEID0000000",
		Code:      "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		CreatedBy: "01ADMINID0000000",
		ExpiresAt: time.Now().Add(72 * time.Hour),
		CreatedAt: time.Now(),
	}
	assert.NoError(t, database.Create(&invite).Error)

	state := models.OIDCState{
		ID:        "01STATEID00000000",
		State:     "randomstate123456789012345678901234567890123456789012345678901234",
		Nonce:     "randomnonce123456789012345678901234567890123456789012345678901234",
		ProviderID: "01PROVIDERID000000",
		InviteCode: "invite-code-123",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		CreatedAt: time.Now(),
	}
	assert.NoError(t, database.Create(&state).Error)

	provider := models.OIDCProviderConfig{
		ID:           "01PROVIDERID000000",
		Name:         "SSO",
		Issuer:       "https://issuer.example.com",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://app.example.com/callback",
		Enabled:      true,
	}
	assert.NoError(t, database.Create(&provider).Error)

	setting := models.AppSetting{
		Key:       "oidc_policy",
		Value:     "open",
		UpdatedAt: time.Now(),
	}
	assert.NoError(t, database.Create(&setting).Error)
}

func TestInit_MigratesServiceAliases(t *testing.T) {
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
		CREATE TABLE service_aliases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			canonical TEXT,
			alias TEXT UNIQUE
		)
	`).Error)
	require.NoError(t, legacyDB.Exec(`INSERT INTO service_aliases (canonical, alias) VALUES (?, ?)`, "traefik", "traefik-proxy").Error)
	require.NoError(t, legacyDB.Exec(`INSERT INTO service_aliases (canonical, alias) VALUES (?, ?)`, "traefik ", " traefik-edge ").Error)
	require.NoError(t, legacyDB.Exec(`INSERT INTO service_aliases (canonical, alias) VALUES (?, ?)`, "", "blank-canonical").Error)
	require.NoError(t, legacyDB.Exec(`INSERT INTO service_aliases (canonical, alias) VALUES (?, ?)`, "   ", "whitespace-canonical").Error)
	require.NoError(t, legacyDB.Exec(`INSERT INTO service_aliases (canonical, alias) VALUES (?, ?)`, "traefik", "").Error)
	require.NoError(t, legacyDB.Exec(`INSERT INTO service_aliases (canonical, alias) VALUES (?, ?)`, "traefik", "   ").Error)
	require.NoError(t, legacyDB.Exec(`INSERT INTO service_aliases (canonical, alias) VALUES (?, ?)`, nil, "null-canonical").Error)
	require.NoError(t, legacyDB.Exec(`INSERT INTO service_aliases (canonical, alias) VALUES (?, ?)`, "traefik", nil).Error)

	sqlDB, err := legacyDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	database, err := db.Init(tmp.Name())
	require.NoError(t, err)

	assert.NoError(t, database.Create(&models.ServiceAlias{Canonical: "traefik", Alias: "traefik-dashboard"}).Error)

	var proxy models.ServiceAlias
	require.NoError(t, database.Where("alias = ?", "traefik-proxy").First(&proxy).Error)
	assert.Equal(t, "traefik", proxy.Canonical)

	var edge models.ServiceAlias
	require.NoError(t, database.Where("alias = ?", "traefik-edge").First(&edge).Error)
	assert.Equal(t, "traefik", edge.Canonical)

	var paddedCount int64
	require.NoError(t, database.Model(&models.ServiceAlias{}).Where("alias = ? OR canonical = ?", " traefik-edge ", "traefik ").Count(&paddedCount).Error)
	assert.Zero(t, paddedCount)

	var invalidCount int64
	require.NoError(t, database.Model(&models.ServiceAlias{}).Where(
		"TRIM(canonical) = '' OR canonical IS NULL OR TRIM(alias) = '' OR alias IS NULL",
	).Count(&invalidCount).Error)
	assert.Zero(t, invalidCount)

	assert.Error(t, database.Create(&models.ServiceAlias{Canonical: "", Alias: "blank-canonical"}).Error)
	assert.Error(t, database.Create(&models.ServiceAlias{Canonical: "   ", Alias: "blank-canonical-whitespace"}).Error)
	assert.Error(t, database.Create(&models.ServiceAlias{Canonical: "traefik", Alias: ""}).Error)
	assert.Error(t, database.Create(&models.ServiceAlias{Canonical: "traefik", Alias: "   "}).Error)
	assert.Error(t, database.Create(&models.ServiceAlias{Canonical: "traefik", Alias: "traefik-edge"}).Error)
}

func TestInit_CreatesMissingDatabasePath(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "nested", "blackbox.db")

	database, err := db.Init(dbPath)
	require.NoError(t, err)
	assert.NotNil(t, database)

	info, err := os.Stat(dbPath)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestInit_ReturnsHelpfulErrorForReadOnlyDirectory(t *testing.T) {
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
