package middleware

import (
	"errors"
	"net/http"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

// TokenVersionCheck validates that the JWT's tv claim matches the user's current
// token_version in the database. Must run after JWTAuth (requires claims in context).
func TokenVersionCheck(database *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := auth.ClaimsFromContext(r.Context())
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			var user models.User
			if err := database.WithContext(r.Context()).First(&user, "id = ?", claims.UserID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					writeJSONError(w, http.StatusUnauthorized, "unauthorized")
					return
				}
				writeJSONError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			if user.TokenVersion != claims.TokenVersion {
				writeJSONError(w, http.StatusUnauthorized, "session invalidated")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
