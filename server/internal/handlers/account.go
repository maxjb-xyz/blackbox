package handlers

import (
	"errors"
	"net/http"
	"strings"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

// UpdateAccount handles PATCH /api/auth/me
// Allows a password-based user to update their email address.
// OIDC-linked accounts cannot change their email this way.
func UpdateAccount(database *gorm.DB, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if claims.OIDCLinked {
			writeError(w, http.StatusForbidden, "email is managed by your SSO provider")
			return
		}

		var req struct {
			Email string `json:"email"`
		}
		if !decodeJSONBody(w, r, maxCredentialBodyBytes, &req) {
			return
		}
		req.Email = strings.TrimSpace(req.Email)

		var user models.User
		if err := database.First(&user, "id = ?", claims.UserID).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load user")
			return
		}

		if user.OIDCIssuer != "" {
			writeError(w, http.StatusForbidden, "email is managed by your SSO provider")
			return
		}

		if req.Email != "" {
			var existing models.User
			err := database.First(&existing, "email = ? AND id != ?", req.Email, user.ID).Error
			if err == nil {
				writeError(w, http.StatusConflict, "email already in use")
				return
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusInternalServerError, "failed to check email")
				return
			}
		}

		if err := database.Model(&user).Update("email", req.Email).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update email")
			return
		}
		user.Email = req.Email

		token, err := auth.IssueJWT(
			user.ID,
			user.Username,
			user.Email,
			false,
			user.IsAdmin,
			user.TokenVersion,
			jwtSecret,
			jwtTTL(),
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}
		setSessionCookie(w, r, token, jwtTTL())

		writeSessionResponse(w, &auth.Claims{
			UserID:       user.ID,
			Username:     user.Username,
			Email:        user.Email,
			OIDCLinked:   false,
			IsAdmin:      user.IsAdmin,
			TokenVersion: user.TokenVersion,
		}, http.StatusOK)
	}
}
