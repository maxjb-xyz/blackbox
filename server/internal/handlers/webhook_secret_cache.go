package handlers

import (
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

const webhookSecretCacheTTL = 60 * time.Second

type cachedSecret struct {
	secret    string
	expiresAt time.Time
}

var (
	webhookSecretCache     sync.Map
	webhookSecretRefreshes singleflight.Group
)

func PrimeWebhookSecretCache(db *gorm.DB, sourceType, envFallback string) string {
	return refreshWebhookSecretCache(db, sourceType, envFallback)
}

// GetCachedWebhookSecret returns the webhook secret for sourceType, reading from
// the DB at most once per webhookSecretCacheTTL. Changes made via the admin UI
// take effect immediately because refreshWebhookSecretCacheIfNeeded is called
// synchronously after every create/update/delete. The TTL is a safety net for
// external DB edits and for expiring stale entries after a delete.
func GetCachedWebhookSecret(db *gorm.DB, sourceType, envFallback string) string {
	if v, ok := webhookSecretCache.Load(sourceType); ok {
		if entry, ok := v.(cachedSecret); ok && time.Now().Before(entry.expiresAt) {
			if entry.secret != "" {
				return entry.secret
			}
			// Empty secret cached — still respect TTL to avoid hammering the DB,
			// but fall through to env fallback.
			return envFallback
		}
		webhookSecretCache.Delete(sourceType)
	}
	return refreshWebhookSecretCache(db, sourceType, envFallback)
}

func refreshWebhookSecretCache(db *gorm.DB, sourceType, envFallback string) string {
	resolved, _, _ := webhookSecretRefreshes.Do(sourceType, func() (interface{}, error) {
		secret := GetWebhookSecret(db, sourceType)
		if secret == "" {
			secret = envFallback
		}
		webhookSecretCache.Store(sourceType, cachedSecret{
			secret:    secret,
			expiresAt: time.Now().Add(webhookSecretCacheTTL),
		})
		return secret, nil
	})
	if secret, ok := resolved.(string); ok {
		return secret
	}
	return envFallback
}

func refreshWebhookSecretCacheIfNeeded(db *gorm.DB, sourceType string) {
	if sourceType == "" || db == nil || !strings.HasPrefix(sourceType, "webhook_") {
		return
	}
	webhookSecretCache.Delete(sourceType)
	refreshWebhookSecretCache(db, sourceType, "")
}

func ResetWebhookSecretCacheForTesting(sourceTypes ...string) {
	for _, sourceType := range sourceTypes {
		webhookSecretCache.Delete(sourceType)
	}
}
