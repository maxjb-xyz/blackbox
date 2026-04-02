package auth_test

import (
	"testing"

	"blackbox/server/internal/auth"
	"github.com/stretchr/testify/assert"
)

func TestNewOIDCProvider_FailsWithBadIssuer(t *testing.T) {
	provider, err := auth.NewOIDCProvider(
		t.Context(),
		"https://does-not-exist.invalid",
		"client-id",
		"client-secret",
		"http://localhost:8080/api/auth/oidc/callback",
	)
	assert.Error(t, err)
	assert.Nil(t, provider)
}
