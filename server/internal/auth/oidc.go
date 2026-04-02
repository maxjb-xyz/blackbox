package auth

import (
	"context"
	"fmt"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDCProvider struct {
	Provider *gooidc.Provider
	Config   oauth2.Config
	Verifier *gooidc.IDTokenVerifier
}

func NewOIDCProvider(ctx context.Context, issuer, clientID, clientSecret, redirectURL string) (*OIDCProvider, error) {
	provider, err := gooidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery failed for %s: %w", issuer, err)
	}

	cfg := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{gooidc.ScopeOpenID, "profile", "email"},
	}
	verifier := provider.Verifier(&gooidc.Config{ClientID: clientID})

	return &OIDCProvider{
		Provider: provider,
		Config:   cfg,
		Verifier: verifier,
	}, nil
}
