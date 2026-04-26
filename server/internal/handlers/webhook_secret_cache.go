package handlers

import (
	"sync"

	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

var (
	webhookSecretCache     sync.Map
	webhookSecretFallbacks sync.Map
	webhookSecretRefreshes singleflight.Group
)

func PrimeWebhookSecretCache(db *gorm.DB, sourceType, envFallback string) string {
	webhookSecretFallbacks.Store(sourceType, envFallback)
	return RefreshWebhookSecretCache(db, sourceType)
}

func GetCachedWebhookSecret(db *gorm.DB, sourceType, envFallback string) string {
	if cached, ok := webhookSecretCache.Load(sourceType); ok {
		if secret, ok := cached.(string); ok {
			return secret
		}
		webhookSecretCache.Delete(sourceType)
	}
	webhookSecretFallbacks.LoadOrStore(sourceType, envFallback)
	return RefreshWebhookSecretCache(db, sourceType)
}

func RefreshWebhookSecretCache(db *gorm.DB, sourceType string) string {
	resolved, _, _ := webhookSecretRefreshes.Do(sourceType, func() (interface{}, error) {
		secret := GetWebhookSecret(db, sourceType)
		webhookSecretCache.Store(sourceType, secret)
		return secret, nil
	})
	if secret, ok := resolved.(string); ok {
		return secret
	}
	return loadWebhookSecretFallback(sourceType)
}

func refreshWebhookSecretCacheIfNeeded(db *gorm.DB, sourceType string) {
	if sourceType == "" || db == nil {
		return
	}
	if _, ok := webhookSecretFallbacks.Load(sourceType); ok {
		RefreshWebhookSecretCache(db, sourceType)
	}
}

func loadWebhookSecretFallback(sourceType string) string {
	if fallback, ok := webhookSecretFallbacks.Load(sourceType); ok {
		if secret, ok := fallback.(string); ok {
			return secret
		}
	}
	return ""
}
