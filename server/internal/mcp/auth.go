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
	writeJSONError(w, http.StatusUnauthorized, "unauthorized")
}

func writeUnavailable(w http.ResponseWriter) {
	writeJSONError(w, http.StatusServiceUnavailable, "mcp is disabled")
}

func writeForbiddenOrigin(w http.ResponseWriter) {
	writeJSONError(w, http.StatusForbidden, "origin not allowed")
}

func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
