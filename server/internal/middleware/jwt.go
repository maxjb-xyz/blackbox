package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"blackbox/server/internal/auth"
)

func JWTAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := ""
			cookie, err := r.Cookie(auth.SessionCookieName)
			if err == nil {
				tokenString = cookie.Value
			}
			if tokenString == "" {
				writeJSONError(w, http.StatusUnauthorized, "missing authorization token")
				return
			}
			claims, err := auth.VerifyJWT(tokenString, secret)
			if err != nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), auth.ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
