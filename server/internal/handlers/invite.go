package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/events"
	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

func inviteTTL() time.Duration {
	if v := os.Getenv("INVITE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 72 * time.Hour
}

func CreateInvite(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok || !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}

		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate code")
			return
		}

		code := hex.EncodeToString(b)
		expiresAt := time.Now().Add(inviteTTL())
		invite := models.InviteCode{
			ID:        ulid.Make().String(),
			Code:      code,
			CreatedBy: claims.UserID,
			ExpiresAt: expiresAt,
			CreatedAt: time.Now(),
		}
		if err := database.Create(&invite).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create invite")
			return
		}

		var admin models.User
		adminName := claims.UserID
		if err := database.First(&admin, "id = ?", claims.UserID).Error; err == nil {
			adminName = admin.Username
		}
		events.LogSystem(database, "auth", "invite.created", "invite created by "+adminName)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"code":       code,
			"expires_at": expiresAt.UTC().Format(time.RFC3339),
		})
	}
}

func ListInvites(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok || !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}

		var invites []models.InviteCode
		if err := database.Where("used_by = '' AND expires_at > ?", time.Now()).Find(&invites).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list invites")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(invites)
	}
}
