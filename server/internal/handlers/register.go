package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/events"
	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

var errInvalidInvite = errors.New("invalid or expired invite")

func Register(database *gorm.DB, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username   string `json:"username"`
			Password   string `json:"password"`
			Email      string `json:"email"`
			InviteCode string `json:"invite_code"`
		}
		if !decodeJSONBody(w, r, maxCredentialBodyBytes, &req) {
			return
		}
		email := strings.TrimSpace(req.Email)
		if req.Username == "" || req.Password == "" || email == "" || req.InviteCode == "" {
			writeError(w, http.StatusBadRequest, "username, password, email, and invite_code required")
			return
		}
		parsedEmail, err := mail.ParseAddress(email)
		if err != nil || parsedEmail.Address != email {
			writeError(w, http.StatusBadRequest, "valid email required")
			return
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}

		userID := ulid.Make().String()
		user := models.User{}
		var token string

		txErr := database.Transaction(func(tx *gorm.DB) error {
			// Atomically claim the invite: conditional UPDATE checks used_by = '' and
			// expiry in one step. RowsAffected == 0 means already used, not found, or expired.
			result := tx.Model(&models.InviteCode{}).
				Where("code = ? AND used_by = '' AND expires_at > ?", req.InviteCode, time.Now()).
				Update("used_by", userID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return errInvalidInvite
			}

			user = models.User{
				ID:           userID,
				Username:     req.Username,
				Email:        email,
				PasswordHash: hash,
				IsAdmin:      false,
				CreatedAt:    time.Now(),
			}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}

			var issueErr error
			token, issueErr = auth.IssueJWT(userID, user.Username, user.Email, false, false, user.TokenVersion, jwtSecret, jwtTTL())
			return issueErr
		})

		if txErr == errInvalidInvite {
			events.LogSystem(database, "auth", "invite.rejected", "registration attempt with invalid invite code")
			writeError(w, http.StatusUnauthorized, "invalid or expired invite code")
			return
		}
		if txErr != nil {
			writeError(w, http.StatusConflict, "registration failed")
			return
		}

		setSessionCookie(w, r, token, jwtTTL())

		events.LogSystem(database, "auth", "user.register", "user "+req.Username+" registered via invite")

		writeSessionResponse(w, &auth.Claims{
			UserID:       userID,
			Username:     req.Username,
			Email:        user.Email,
			OIDCLinked:   false,
			IsAdmin:      false,
			TokenVersion: user.TokenVersion,
		}, http.StatusCreated)
	}
}
