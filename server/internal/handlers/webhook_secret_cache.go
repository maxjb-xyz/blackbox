package handlers

import (
	"sync"

	"gorm.io/gorm"
)

var (
	webhookSecretCache     sync.Map
	webhookSecretFallbacks sync.Map
)

func PrimeWebhookSecretCache(db *gorm.DB, sourceType, envFallback string) string {
	webhookSecretFallbacks.Store(sourceType, envFallback)
	return RefreshWebhookSecretCache(db, sourceType)
}

func GetCachedWebhookSecret(db *gorm.DB, sourceType, envFallback string) string {
	webhookSecretFallbacks.Store(sourceType, envFallback)
	if cached, ok := webhookSecretCache.Load(sourceType); ok {
		return cached.(string)
	}
	return RefreshWebhookSecretCache(db, sourceType)
}

func RefreshWebhookSecretCache(db *gorm.DB, sourceType string) string {
	envFallback := ""
	if fallback, ok := webhookSecretFallbacks.Load(sourceType); ok {
		envFallback = fallback.(string)
	}
	secret := GetWebhookSecret(db, sourceType, envFallback)
	webhookSecretCache.Store(sourceType, secret)
	return secret
}

func refreshWebhookSecretCacheIfNeeded(db *gorm.DB, sourceType string) {
	if sourceType == "" || db == nil {
		return
	}
	if _, ok := webhookSecretFallbacks.Load(sourceType); ok {
		RefreshWebhookSecretCache(db, sourceType)
	}
}
