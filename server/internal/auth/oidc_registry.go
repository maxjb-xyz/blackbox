package auth

import (
	"context"
	"log"
	"sync"
	"time"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

type OIDCRegistry struct {
	mu        sync.RWMutex
	providers map[string]*OIDCProvider
	db        *gorm.DB
}

func NewOIDCRegistry(db *gorm.DB) *OIDCRegistry {
	return &OIDCRegistry{
		providers: make(map[string]*OIDCProvider),
		db:        db,
	}
}

func (r *OIDCRegistry) Get(id string) *OIDCProvider {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[id]
}

func (r *OIDCRegistry) Reload(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	providerConfigs, err := r.ListEnabled()
	if err != nil {
		return err
	}

	nextProviders := make(map[string]*OIDCProvider, len(providerConfigs))
	for _, providerConfig := range providerConfigs {
		providerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		provider, err := NewOIDCProvider(
			providerCtx,
			providerConfig.Issuer,
			providerConfig.ClientID,
			providerConfig.ClientSecret,
			providerConfig.RedirectURL,
		)
		cancel()
		if err != nil {
			log.Printf("oidc registry warning: provider %s unavailable: %v", providerConfig.ID, err)
			continue
		}
		nextProviders[providerConfig.ID] = provider
	}

	r.mu.Lock()
	r.providers = nextProviders
	r.mu.Unlock()

	return nil
}

func (r *OIDCRegistry) ListEnabled() ([]models.OIDCProviderConfig, error) {
	var providers []models.OIDCProviderConfig
	if err := r.db.Where("enabled = ?", true).Order("created_at ASC").Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}
