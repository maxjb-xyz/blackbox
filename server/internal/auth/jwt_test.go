package auth_test

import (
	"testing"
	"time"

	"blackbox/server/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueAndVerifyJWT_RoundTrip(t *testing.T) {
	token, err := auth.IssueJWT("user-123", "alice", true, "test-secret", time.Hour)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := auth.VerifyJWT(token, "test-secret")
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "alice", claims.Username)
	assert.True(t, claims.IsAdmin)
}

func TestVerifyJWT_WrongSecret(t *testing.T) {
	token, _ := auth.IssueJWT("user-123", "alice", false, "secret-a", time.Hour)
	_, err := auth.VerifyJWT(token, "secret-b")
	assert.Error(t, err)
}

func TestVerifyJWT_Expired(t *testing.T) {
	token, _ := auth.IssueJWT("user-123", "alice", false, "secret", -time.Second)
	_, err := auth.VerifyJWT(token, "secret")
	assert.Error(t, err)
}
