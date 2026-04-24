package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/hub"
	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type adminUserResponse struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	IsAdmin      bool      `json:"is_admin"`
	TokenVersion int       `json:"token_version"`
	CreatedAt    time.Time `json:"created_at"`
}

func toAdminUserResponse(u models.User) adminUserResponse {
	return adminUserResponse{
		ID:           u.ID,
		Username:     u.Username,
		IsAdmin:      u.IsAdmin,
		TokenVersion: u.TokenVersion,
		CreatedAt:    u.CreatedAt,
	}
}

func ListAdminUsers(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var users []models.User
		if err := database.Order("created_at ASC").Find(&users).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list users")
			return
		}
		resp := make([]adminUserResponse, len(users))
		for i, u := range users {
			resp[i] = toAdminUserResponse(u)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("ListAdminUsers encode: %v", err)
		}
	}
}

func UpdateAdminUser(database *gorm.DB, h *hub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerClaims, _ := auth.ClaimsFromContext(r.Context())
		targetID := chi.URLParam(r, "id")

		var req struct {
			IsAdmin *bool `json:"is_admin"`
		}
		if !decodeJSONBody(w, r, 8<<10, &req) {
			return
		}
		if req.IsAdmin == nil {
			writeError(w, http.StatusBadRequest, "missing is_admin")
			return
		}

		// Cannot demote self
		if targetID == callerClaims.UserID && !*req.IsAdmin {
			writeError(w, http.StatusForbidden, "cannot demote yourself")
			return
		}

		var user models.User
		roleChanged := false
		if err := database.Transaction(func(tx *gorm.DB) error {
			if err := tx.First(&user, "id = ?", targetID).Error; err != nil {
				return err
			}

			roleChanged = user.IsAdmin != *req.IsAdmin
			updates := map[string]interface{}{
				"is_admin": *req.IsAdmin,
			}
			if roleChanged {
				updates["token_version"] = gorm.Expr("token_version + 1")
			}
			if err := tx.Model(&user).Updates(updates).Error; err != nil {
				return err
			}

			user.IsAdmin = *req.IsAdmin
			if roleChanged {
				user.TokenVersion++
			}
			return nil
		}); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to update user")
			return
		}

		if roleChanged && h != nil {
			h.InvalidateUser(targetID)
		}

		WriteAuditLog(database, r, callerClaims, "user.update_role", "user", targetID, map[string]interface{}{
			"is_admin":     *req.IsAdmin,
			"role_changed": roleChanged,
		})

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(toAdminUserResponse(user)); err != nil {
			log.Printf("UpdateAdminUser encode: %v", err)
		}
	}
}

func ForceLogoutUser(database *gorm.DB, h *hub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetID := chi.URLParam(r, "id")

		var user models.User
		if err := database.First(&user, "id = ?", targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to fetch user")
			return
		}

		if err := database.Model(&user).UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to invalidate sessions")
			return
		}
		if h != nil {
			h.InvalidateUser(targetID)
		}

		callerClaims, _ := auth.ClaimsFromContext(r.Context())
		WriteAuditLog(database, r, callerClaims, "user.force_logout", "user", targetID, map[string]interface{}{
			"username": user.Username,
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

func DeleteAdminUser(database *gorm.DB, h *hub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerClaims, _ := auth.ClaimsFromContext(r.Context())
		targetID := chi.URLParam(r, "id")

		if targetID == callerClaims.UserID {
			writeError(w, http.StatusForbidden, "cannot delete yourself")
			return
		}

		result := database.Delete(&models.User{}, "id = ?", targetID)
		if result.Error != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete user")
			return
		}
		if result.RowsAffected == 0 {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		if h != nil {
			h.InvalidateUser(targetID)
		}

		WriteAuditLog(database, r, callerClaims, "user.delete", "user", targetID, map[string]interface{}{})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}
