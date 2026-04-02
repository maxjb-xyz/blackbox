package db_test

import (
	"os"
	"testing"

	"blackbox/server/internal/db"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
