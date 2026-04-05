package main

import (
	"fmt"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/models"
)

type oidcRegistryStatus int

const (
	oidcRegistryStatusUnknown oidcRegistryStatus = iota
	oidcRegistryStatusDisabled
	oidcRegistryStatusReady
	oidcRegistryStatusUnavailable
)

func oidcRegistryStatusFromProviders(providers []models.OIDCProviderConfig, registry *auth.OIDCRegistry) oidcRegistryStatus {
	if len(providers) == 0 {
		return oidcRegistryStatusDisabled
	}

	for _, provider := range providers {
		if registry.Get(provider.ID) != nil {
			return oidcRegistryStatusReady
		}
	}

	return oidcRegistryStatusUnavailable
}

func oidcRegistryTransitionMessage(attempt int, previous, current oidcRegistryStatus) string {
	if current == previous {
		return ""
	}

	switch current {
	case oidcRegistryStatusReady:
		return "OIDC registry ready"
	case oidcRegistryStatusUnavailable:
		return fmt.Sprintf("OIDC registry reload attempt %d completed but no providers are currently available", attempt)
	default:
		return ""
	}
}
