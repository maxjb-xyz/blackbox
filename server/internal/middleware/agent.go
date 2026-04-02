package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func AgentAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			incoming := strings.TrimPrefix(header, "Bearer ")
			if subtle.ConstantTimeCompare([]byte(incoming), []byte(token)) != 1 {
				writeJSONError(w, http.StatusUnauthorized, "invalid agent token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
