package handlers

import (
	"testing"

	"blackbox/server/internal/db"
	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindOIDCUserByEmail_RequiresVerifiedEmailWhenConfigured(t *testing.T) {
	database, err := db.Init(":memory:")
	require.NoError(t, err)
	require.NoError(t, database.Create(&models.User{
		ID:       "user-1",
		Username: "alice",
		Email:    "alice@example.com",
	}).Error)

	user, linkExisting, err := findOIDCUserByEmail(database, "https://issuer.example.com", oidcIDClaims{
		Email:         "alice@example.com",
		EmailVerified: false,
	}, true)
	require.NoError(t, err)
	assert.False(t, linkExisting)
	assert.Equal(t, models.User{}, user)

	user, linkExisting, err = findOIDCUserByEmail(database, "https://issuer.example.com", oidcIDClaims{
		Email:         "alice@example.com",
		EmailVerified: false,
	}, false)
	require.NoError(t, err)
	assert.True(t, linkExisting)
	assert.Equal(t, "user-1", user.ID)
}
