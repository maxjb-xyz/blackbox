package middleware

import (
	"crypto/subtle"
	"net/http"
)

func AgentAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			incoming := r.Header.Get("X-Lablog-Agent-Key")
			if subtle.ConstantTimeCompare([]byte(incoming), []byte(token)) != 1 {
				writeJSONError(w, http.StatusUnauthorized, "invalid agent token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
