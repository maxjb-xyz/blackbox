package middleware

import (
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
