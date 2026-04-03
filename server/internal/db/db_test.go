package db_test

import (
	"os"
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

func TestInit_CreatesTablesAndMigrates(t *testing.T) {
	tmp, err := os.CreateTemp("", "blackbox-test-*.db")
	require.NoError(t, err)
	tmp.Close()
	defer os.Remove(tmp.Name())

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

func TestInit_MigratesInviteCodeAndOIDCState(t *testing.T) {
	tmp, err := os.CreateTemp("", "blackbox-test-*.db")
	require.NoError(t, err)
	tmp.Close()
	defer os.Remove(tmp.Name())

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
		ExpiresAt: time.Now().Add(10 * time.Minute),
		CreatedAt: time.Now(),
	}
	assert.NoError(t, database.Create(&state).Error)
}

func TestInit_MigratesServiceAliases(t *testing.T) {
	tmp, err := os.CreateTemp("", "blackbox-test-*.db")
	require.NoError(t, err)
	tmp.Close()
	defer os.Remove(tmp.Name())

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

	assert.NoError(t, database.Create(&models.ServiceAlias{Canonical: "traefik", Alias: "traefik-edge"}).Error)

	var proxy models.ServiceAlias
	require.NoError(t, database.Where("alias = ?", "traefik-proxy").First(&proxy).Error)
	assert.Equal(t, "traefik", proxy.Canonical)

	var edge models.ServiceAlias
	require.NoError(t, database.Where("alias = ?", "traefik-edge").First(&edge).Error)
	assert.Equal(t, "traefik", edge.Canonical)

	var invalidCount int64
	require.NoError(t, database.Model(&models.ServiceAlias{}).Where(
		"TRIM(canonical) = '' OR canonical IS NULL OR TRIM(alias) = '' OR alias IS NULL",
	).Count(&invalidCount).Error)
	assert.Zero(t, invalidCount)

	assert.Error(t, database.Create(&models.ServiceAlias{Canonical: "", Alias: "blank-canonical"}).Error)
	assert.Error(t, database.Create(&models.ServiceAlias{Canonical: "   ", Alias: "blank-canonical-whitespace"}).Error)
	assert.Error(t, database.Create(&models.ServiceAlias{Canonical: "traefik", Alias: ""}).Error)
	assert.Error(t, database.Create(&models.ServiceAlias{Canonical: "traefik", Alias: "   "}).Error)
}
