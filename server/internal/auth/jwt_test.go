package auth_test

import (
	"testing"
	"time"

	"blackbox/server/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueAndVerifyJWT_RoundTrip(t *testing.T) {
	token, err := auth.IssueJWT("user-123", "alice", "alice@example.com", false, true, 0, "test-secret", time.Hour)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := auth.VerifyJWT(token, "test-secret")
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "alice", claims.Username)
	assert.Equal(t, "alice@example.com", claims.Email)
	assert.False(t, claims.OIDCLinked)
	assert.True(t, claims.IsAdmin)
	assert.Equal(t, 0, claims.TokenVersion)
}

func TestVerifyJWT_WrongSecret(t *testing.T) {
	token, err := auth.IssueJWT("user-123", "alice", "", false, false, 0, "secret-a", time.Hour)
	require.NoError(t, err)
	_, err = auth.VerifyJWT(token, "secret-b")
	assert.Error(t, err)
}

func TestVerifyJWT_Expired(t *testing.T) {
	token, err := auth.IssueJWT("user-123", "alice", "", false, false, 0, "secret", -time.Second)
	require.NoError(t, err)
	_, err = auth.VerifyJWT(token, "secret")
	assert.Error(t, err)
}
