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
	ready     bool
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

func (r *OIDCRegistry) IsReady() bool {
	if r == nil {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready
}

func (r *OIDCRegistry) SetProvider(id string, provider *OIDCProvider) {
	if r == nil || id == "" || provider == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[id] = provider
	r.ready = true
}

func (r *OIDCRegistry) Reload(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	providerConfigs, err := r.ListEnabled()
	if err != nil {
		return err
	}

	r.mu.Lock()
	liveProviders := make(map[string]*OIDCProvider, len(r.providers))
	nextProviders := make(map[string]*OIDCProvider, len(r.providers))
	for id, provider := range r.providers {
		liveProviders[id] = provider
		nextProviders[id] = provider
	}
	r.mu.Unlock()

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
			if nextProviders[providerConfig.ID] != nil {
				log.Printf("oidc registry warning: retaining previous provider %s after failed refresh", providerConfig.ID)
			}
			continue
		}
		nextProviders[providerConfig.ID] = provider
	}

	enabledIDs := make(map[string]struct{}, len(providerConfigs))
	for _, providerConfig := range providerConfigs {
		enabledIDs[providerConfig.ID] = struct{}{}
	}
	for id := range nextProviders {
		if _, ok := enabledIDs[id]; !ok {
			delete(nextProviders, id)
		}
	}

	r.mu.Lock()
	for id, provider := range r.providers {
		if liveProviders[id] != provider && nextProviders[id] == liveProviders[id] {
			nextProviders[id] = provider
		}
	}
	r.providers = nextProviders
	r.ready = true
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
