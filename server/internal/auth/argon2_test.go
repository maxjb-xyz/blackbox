package auth_test

import (
	"testing"

	"blackbox/server/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword_ProducesArgon2idFormat(t *testing.T) {
	hash, err := auth.HashPassword("hunter2")
	require.NoError(t, err)
	assert.Contains(t, hash, "$argon2id$")
}

func TestVerifyPassword_CorrectPassword(t *testing.T) {
	hash, err := auth.HashPassword("correct-horse")
	require.NoError(t, err)
	assert.True(t, auth.VerifyPassword(hash, "correct-horse"))
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, err := auth.HashPassword("correct-horse")
	require.NoError(t, err)
	assert.False(t, auth.VerifyPassword(hash, "wrong-password"))
}

func TestVerifyPassword_TwoHashesDiffer(t *testing.T) {
	h1, _ := auth.HashPassword("same")
	h2, _ := auth.HashPassword("same")
	assert.NotEqual(t, h1, h2)
	assert.True(t, auth.VerifyPassword(h1, "same"))
	assert.True(t, auth.VerifyPassword(h2, "same"))
}
