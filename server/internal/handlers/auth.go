package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

func jwtTTL() time.Duration {
	if v := os.Getenv("JWT_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 24 * time.Hour
}

func Bootstrap(database *gorm.DB, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var count int64
		database.Model(&models.User{}).Count(&count)
		if count > 0 {
			writeError(w, http.StatusConflict, "already bootstrapped")
			return
		}

		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
			writeError(w, http.StatusBadRequest, "username and password required")
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
			IsAdmin:      true,
		}
		if err := database.Create(&user).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create user")
			return
		}

		token, err := auth.IssueJWT(user.ID, user.IsAdmin, jwtSecret, jwtTTL())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"token": token})
	}
}

func Login(database *gorm.DB, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		var user models.User
		if err := database.First(&user, "username = ?", req.Username).Error; err != nil {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}

		if !auth.VerifyPassword(user.PasswordHash, req.Password) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}

		token, err := auth.IssueJWT(user.ID, user.IsAdmin, jwtSecret, jwtTTL())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": token})
	}
}

func OIDCStub() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotImplemented, "OIDC not configured")
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
