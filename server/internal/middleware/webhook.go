package middleware

import (
	"crypto/subtle"
	"net/http"
)

// WebhookAuth validates the dedicated webhook secret header.
func WebhookAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
