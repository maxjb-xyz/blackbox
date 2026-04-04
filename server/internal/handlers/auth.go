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
	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxCredentialBodyBytes = 8 << 10
)

var errAlreadyBootstrapped = errors.New("already bootstrapped")

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
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !decodeJSONBody(w, r, maxCredentialBodyBytes, &req) {
			return
		}
		if req.Username == "" || req.Password == "" {
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
		var token string

		txErr := database.Transaction(func(tx *gorm.DB) error {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.SetupState{Key: "bootstrap"})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return errAlreadyBootstrapped
			}

			var count int64
			if err := tx.Model(&models.User{}).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errAlreadyBootstrapped
			}

			if err := tx.Create(&user).Error; err != nil {
				return err
			}

			var issueErr error
			token, issueErr = auth.IssueJWT(user.ID, user.Username, user.IsAdmin, jwtSecret, jwtTTL())
			return issueErr
		})
		if txErr == errAlreadyBootstrapped {
			writeError(w, http.StatusConflict, "already bootstrapped")
			return
		}
		if txErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to create user")
			return
		}

		setSessionCookie(w, r, token, jwtTTL())

		events.LogSystem(database, "auth", "admin.bootstrap", "admin user "+req.Username+" created")

		writeSessionResponse(w, &auth.Claims{
			UserID:   user.ID,
			Username: user.Username,
			IsAdmin:  user.IsAdmin,
		}, http.StatusCreated)
	}
}

func Login(database *gorm.DB, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !decodeJSONBody(w, r, maxCredentialBodyBytes, &req) {
			return
		}

		var user models.User
		if err := database.First(&user, "username = ?", req.Username).Error; err != nil {
			events.LogSystem(database, "auth", "user.login.failed", "failed login attempt for username "+req.Username)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}

		if !auth.VerifyPassword(user.PasswordHash, req.Password) {
			events.LogSystem(database, "auth", "user.login.failed", "failed login attempt for username "+req.Username)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}

		token, err := auth.IssueJWT(user.ID, user.Username, user.IsAdmin, jwtSecret, jwtTTL())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		setSessionCookie(w, r, token, jwtTTL())

		events.LogSystem(database, "auth", "user.login", "user "+req.Username+" logged in")

		writeSessionResponse(w, &auth.Claims{
			UserID:   user.ID,
			Username: user.Username,
			IsAdmin:  user.IsAdmin,
		}, http.StatusOK)
	}
}

func OIDCLogin(database *gorm.DB, oidcProvider *auth.OIDCProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if oidcProvider == nil {
			writeError(w, http.StatusServiceUnavailable, "OIDC provider unavailable")
			return
		}

		stateBytes := make([]byte, 32)
		nonceBytes := make([]byte, 32)
		if _, err := rand.Read(stateBytes); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate state")
			return
		}
		if _, err := rand.Read(nonceBytes); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate nonce")
			return
		}

		state := hex.EncodeToString(stateBytes)
		nonce := hex.EncodeToString(nonceBytes)
		oidcState := models.OIDCState{
			ID:        ulid.Make().String(),
			State:     state,
			Nonce:     nonce,
			ExpiresAt: time.Now().Add(10 * time.Minute),
			CreatedAt: time.Now(),
		}
		if err := database.Create(&oidcState).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store state")
			return
		}

		stateCookie := &http.Cookie{
			Name:     "oidc_state",
			Value:    state,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   600,
			Path:     "/",
		}
		if isSecureRequest(r) {
			stateCookie.Secure = true
		}
		http.SetCookie(w, stateCookie)

		http.Redirect(w, r, oidcProvider.Config.AuthCodeURL(state, gooidc.Nonce(nonce)), http.StatusFound)
	}
}

func OIDCCallback(database *gorm.DB, oidcProvider *auth.OIDCProvider, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if oidcProvider == nil {
			writeError(w, http.StatusServiceUnavailable, "OIDC provider unavailable")
			return
		}

		cookie, err := r.Cookie("oidc_state")
		if err != nil {
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: missing state cookie")
			writeError(w, http.StatusBadRequest, "missing state cookie")
			return
		}

		if r.URL.Query().Get("state") != cookie.Value {
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: state mismatch")
			writeError(w, http.StatusBadRequest, "state mismatch")
			return
		}

		var oidcState models.OIDCState
		if err := database.First(&oidcState, "state = ?", cookie.Value).Error; err != nil || oidcState.ExpiresAt.Before(time.Now()) {
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: invalid or expired state")
			writeError(w, http.StatusBadRequest, "invalid or expired state")
			return
		}

		oauth2Token, err := oidcProvider.Config.Exchange(r.Context(), r.URL.Query().Get("code"))
		if err != nil {
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: code exchange failed")
			writeError(w, http.StatusUnauthorized, "code exchange failed")
			return
		}

		rawIDToken, ok := oauth2Token.Extra("id_token").(string)
		if !ok {
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: missing id_token")
			writeError(w, http.StatusUnauthorized, "missing id_token")
			return
		}

		idToken, err := oidcProvider.Verifier.Verify(r.Context(), rawIDToken)
		if err != nil {
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: id_token verification failed")
			writeError(w, http.StatusUnauthorized, "id_token verification failed")
			return
		}

		var claims struct {
			Nonce string `json:"nonce"`
			Sub   string `json:"sub"`
		}
		if err := idToken.Claims(&claims); err != nil || claims.Nonce != oidcState.Nonce {
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: nonce mismatch")
			writeError(w, http.StatusBadRequest, "nonce mismatch")
			return
		}

		database.Delete(&oidcState)

		var user models.User
		result := database.First(&user, "oidc_subject = ?", claims.Sub)
		if result.Error != nil {
			user = models.User{
				ID:          ulid.Make().String(),
				Username:    claims.Sub,
				OIDCSubject: claims.Sub,
				IsAdmin:     false,
				CreatedAt:   time.Now(),
			}
			if err := database.Create(&user).Error; err != nil {
				writeError(w, http.StatusInternalServerError, "failed to create user")
				return
			}
		}

		token, err := auth.IssueJWT(user.ID, user.Username, user.IsAdmin, jwtSecret, jwtTTL())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		setSessionCookie(w, r, token, jwtTTL())
		http.SetCookie(w, &http.Cookie{
			Name:     "oidc_state",
			Value:    "",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			Secure:   isSecureRequest(r),
		})

		events.LogSystem(database, "auth", "user.login.oidc", "user "+claims.Sub+" logged in via OIDC")

		http.Redirect(w, r, "/timeline", http.StatusFound)
	}
}

func CurrentUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		writeSessionResponse(w, claims, http.StatusOK)
	}
}

func Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clearSessionCookie(w, r)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
