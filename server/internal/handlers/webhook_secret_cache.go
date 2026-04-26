package handlers

import (
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

var (
	webhookSecretCache     sync.Map
	webhookSecretRefreshes singleflight.Group
)

func PrimeWebhookSecretCache(db *gorm.DB, sourceType string) string {
	return RefreshWebhookSecretCache(db, sourceType)
}

func GetCachedWebhookSecret(db *gorm.DB, sourceType string) string {
	if cached, ok := webhookSecretCache.Load(sourceType); ok {
		if secret, ok := cached.(string); ok {
			return secret
		}
		webhookSecretCache.Delete(sourceType)
	}
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
	return ""
}

func refreshWebhookSecretCacheIfNeeded(db *gorm.DB, sourceType string) {
	if sourceType == "" || db == nil || !strings.HasPrefix(sourceType, "webhook_") {
		return
	}
	RefreshWebhookSecretCache(db, sourceType)
}

func ResetWebhookSecretCacheForTesting(sourceTypes ...string) {
	for _, sourceType := range sourceTypes {
		webhookSecretCache.Delete(sourceType)
	}
}
