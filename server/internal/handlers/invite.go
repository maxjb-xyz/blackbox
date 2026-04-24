package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/events"
	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
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

		WriteAuditLog(database, r, claims, "invite.create", "invite", invite.ID, map[string]interface{}{
			"expires_at": expiresAt.UTC().Format(time.RFC3339),
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
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
		_ = json.NewEncoder(w).Encode(invites)
	}
}

func RevokeInvite(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok || !claims.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}

		inviteID := chi.URLParam(r, "id")
		result := database.Where("id = ? AND used_by = ''", inviteID).Delete(&models.InviteCode{})
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to revoke invite")
			return
		}
		if result.RowsAffected == 0 {
			var invite models.InviteCode
			if err := database.First(&invite, "id = ?", inviteID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					writeError(w, http.StatusNotFound, "invite not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to fetch invite")
				return
			}
			if invite.UsedBy != "" {
				writeError(w, http.StatusConflict, "invite already used")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to revoke invite")
			return
		}

		events.LogSystem(database, "auth", "invite.revoked", "invite "+inviteID+" revoked")

		WriteAuditLog(database, r, claims, "invite.revoke", "invite", inviteID, map[string]interface{}{})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}
