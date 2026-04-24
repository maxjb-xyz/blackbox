package mcp

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

func BearerTokenMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		incoming := strings.TrimPrefix(authHeader, "Bearer ")
		if incoming == "" || authHeader == incoming {
			writeUnauthorized(w)
			return
		}
		if subtle.ConstantTimeCompare([]byte(incoming), []byte(token)) != 1 {
			writeUnauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
