package middleware

import (
	"net/http"

	"blackbox/server/internal/auth"
)

// RequireAdmin rejects requests where the JWT claims do not have IsAdmin=true.
// Must run after JWTAuth (requires claims in context).
func RequireAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := auth.ClaimsFromContext(r.Context())
			if !ok || !claims.IsAdmin {
				writeJSONError(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
