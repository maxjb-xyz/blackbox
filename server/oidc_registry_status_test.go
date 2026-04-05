package main

import (
	"testing"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/db"
	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOIDCRegistryStatusFromProviders(t *testing.T) {
	t.Run("disabled when no providers are enabled", func(t *testing.T) {
		registry := newTestOIDCRegistry(t)

		status := oidcRegistryStatusFromProviders(nil, registry)

		assert.Equal(t, oidcRegistryStatusDisabled, status)
	})

	t.Run("ready when an enabled provider is live", func(t *testing.T) {
		registry := newTestOIDCRegistry(t)
		registry.SetProvider("provider-1", &auth.OIDCProvider{})

		status := oidcRegistryStatusFromProviders([]models.OIDCProviderConfig{
			{ID: "provider-1"},
		}, registry)

		assert.Equal(t, oidcRegistryStatusReady, status)
	})

	t.Run("unavailable when enabled providers exist but none are live", func(t *testing.T) {
		registry := newTestOIDCRegistry(t)

		status := oidcRegistryStatusFromProviders([]models.OIDCProviderConfig{
			{ID: "provider-1"},
		}, registry)

		assert.Equal(t, oidcRegistryStatusUnavailable, status)
	})
}

func TestOIDCRegistryTransitionMessage(t *testing.T) {
	assert.Equal(t, "", oidcRegistryTransitionMessage(2, oidcRegistryStatusReady, oidcRegistryStatusReady))
	assert.Equal(t, "OIDC registry ready", oidcRegistryTransitionMessage(1, oidcRegistryStatusDisabled, oidcRegistryStatusReady))
	assert.Equal(
		t,
		"OIDC registry reload attempt 3 completed but no providers are currently available",
		oidcRegistryTransitionMessage(3, oidcRegistryStatusReady, oidcRegistryStatusUnavailable),
	)
	assert.Equal(t, "", oidcRegistryTransitionMessage(4, oidcRegistryStatusUnavailable, oidcRegistryStatusDisabled))
}

func newTestOIDCRegistry(t *testing.T) *auth.OIDCRegistry {
	t.Helper()

	database, err := db.Init(":memory:")
	require.NoError(t, err)

	return auth.NewOIDCRegistry(database)
}
