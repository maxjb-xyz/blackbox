package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
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
			InviteCode string `json:"invite_code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" || req.InviteCode == "" {
			writeError(w, http.StatusBadRequest, "username, password, and invite_code required")
			return
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}

		userID := ulid.Make().String()
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

			user := models.User{
				ID:           userID,
				Username:     req.Username,
				PasswordHash: hash,
				IsAdmin:      false,
				CreatedAt:    time.Now(),
			}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}

			var issueErr error
			token, issueErr = auth.IssueJWT(userID, false, jwtSecret, jwtTTL())
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

		events.LogSystem(database, "auth", "user.register", "user "+req.Username+" registered via invite")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"token": token})
	}
}
