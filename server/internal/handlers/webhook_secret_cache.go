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

type cachedSecretMap struct {
	secrets   map[string]string
	expiresAt time.Time
}

var (
	webhookSecretCache      sync.Map
	webhookSecretRefreshes  singleflight.Group
	webhookSecretsRefreshes singleflight.Group
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

// GetCachedWebhookSecrets returns a map of sourceID → secret for all enabled
// instances of sourceType. Uses the same TTL and invalidation logic as the
// singleton cache. Returns an empty map (never nil) if no instances are found.
// The returned map is a copy; callers may not mutate the cache through it.
func GetCachedWebhookSecrets(db *gorm.DB, sourceType string) map[string]string {
	key := sourceType + ":multi"
	if v, ok := webhookSecretCache.Load(key); ok {
		if entry, ok := v.(cachedSecretMap); ok && time.Now().Before(entry.expiresAt) {
			return cloneStringMap(entry.secrets)
		}
		webhookSecretCache.Delete(key)
	}
	return refreshWebhookSecretsCache(db, sourceType)
}

// PrimeWebhookSecretsCache eagerly loads the multi-instance secret cache for
// sourceType at startup so the first inbound request does not hit the DB.
func PrimeWebhookSecretsCache(db *gorm.DB, sourceType string) map[string]string {
	return refreshWebhookSecretsCache(db, sourceType)
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

func refreshWebhookSecretsCache(db *gorm.DB, sourceType string) map[string]string {
	key := sourceType + ":multi"
	resolved, _, _ := webhookSecretsRefreshes.Do(sourceType, func() (interface{}, error) {
		secrets := GetAllWebhookSecrets(db, sourceType)
		webhookSecretCache.Store(key, cachedSecretMap{
			secrets:   cloneStringMap(secrets),
			expiresAt: time.Now().Add(webhookSecretCacheTTL),
		})
		return secrets, nil
	})
	if secrets, ok := resolved.(map[string]string); ok {
		return cloneStringMap(secrets)
	}
	return map[string]string{}
}

func cloneStringMap(m map[string]string) map[string]string {
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func refreshWebhookSecretCacheIfNeeded(db *gorm.DB, sourceType string) {
	if sourceType == "" || db == nil || !strings.HasPrefix(sourceType, "webhook_") {
		return
	}
	webhookSecretCache.Delete(sourceType)
	webhookSecretCache.Delete(sourceType + ":multi")
	refreshWebhookSecretCache(db, sourceType, "")
	refreshWebhookSecretsCache(db, sourceType)
}

func ResetWebhookSecretCacheForTesting(sourceTypes ...string) {
	for _, sourceType := range sourceTypes {
		webhookSecretCache.Delete(sourceType)
		webhookSecretCache.Delete(sourceType + ":multi")
	}
}
