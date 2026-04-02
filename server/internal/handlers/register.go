package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/events"
	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

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

		var invite models.InviteCode
		if err := database.First(&invite, "code = ?", req.InviteCode).Error; err != nil || invite.UsedBy != "" || invite.ExpiresAt.Before(time.Now()) {
			events.LogSystem(database, "auth", "invite.rejected", "registration attempt with invalid invite code")
			writeError(w, http.StatusUnauthorized, "invalid or expired invite code")
			return
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}

		user := models.User{
			ID:           ulid.Make().String(),
			Username:     req.Username,
			PasswordHash: hash,
			IsAdmin:      false,
			CreatedAt:    time.Now(),
		}
		if err := database.Create(&user).Error; err != nil {
			writeError(w, http.StatusConflict, "username already exists")
			return
		}

		if err := database.Model(&invite).Update("used_by", user.ID).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to redeem invite")
			return
		}

		token, err := auth.IssueJWT(user.ID, user.IsAdmin, jwtSecret, jwtTTL())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		events.LogSystem(database, "auth", "user.register", "user "+req.Username+" registered via invite")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"token": token})
	}
}
