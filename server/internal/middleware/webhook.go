package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
)

// WebhookAuthFunc returns middleware that validates the webhook secret
// by calling getSecret() on each request, enabling runtime secret rotation.
func WebhookAuthFunc(getSecret func() string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			secret := getSecret()
			if secret == "" {
				writeJSONError(w, http.StatusUnauthorized, "server webhook secret not configured")
				return
			}
			incoming := r.Header.Get("X-Webhook-Secret")
			if subtle.ConstantTimeCompare([]byte(incoming), []byte(secret)) != 1 {
				writeJSONError(w, http.StatusUnauthorized, "invalid webhook secret")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// WebhookAuth validates using a static secret string (kept for backward compat and tests).
func WebhookAuth(secret string) func(http.Handler) http.Handler {
	return WebhookAuthFunc(func() string { return secret })
}

type webhookSourceIDKeyType struct{}

var webhookSourceIDKey = webhookSourceIDKeyType{}

// WebhookSourceIDKey returns the context key used by WebhookAuthFuncMulti.
// Exported for use in tests that need to inject a source ID directly.
func WebhookSourceIDKey() any {
	return webhookSourceIDKey
}

// WebhookSourceIDFromContext retrieves the matched source ID injected by
// WebhookAuthFuncMulti. Returns ("", false) if not present.
func WebhookSourceIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(webhookSourceIDKey).(string)
	return id, ok && id != ""
}

// WebhookAuthFuncMulti validates the incoming X-Webhook-Secret header against
// all secrets returned by getSecrets. On a match, it injects the matched
// source ID into the request context so handlers can load per-instance config.
func WebhookAuthFuncMulti(getSecrets func() map[string]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			secrets := getSecrets()
			if len(secrets) == 0 {
				writeJSONError(w, http.StatusUnauthorized, "server webhook secret not configured")
				return
			}
			incoming := []byte(r.Header.Get("X-Webhook-Secret"))
			var matched []string
			for sourceID, secret := range secrets {
				if secret == "" {
					continue
				}
				if subtle.ConstantTimeCompare(incoming, []byte(secret)) == 1 {
					matched = append(matched, sourceID)
				}
			}
			if len(matched) != 1 {
				writeJSONError(w, http.StatusUnauthorized, "invalid webhook secret")
				return
			}
			ctx := context.WithValue(r.Context(), webhookSourceIDKey, matched[0])
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
