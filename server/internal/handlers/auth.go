package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/events"
	"blackbox/server/internal/models"
	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxCredentialBodyBytes = 8 << 10
)

var errAlreadyBootstrapped = errors.New("already bootstrapped")
var errOIDCEmailAmbiguous = errors.New("oidc email match ambiguous")
var errOIDCEmailAlreadyLinked = errors.New("oidc email already linked to a different identity")

type oidcIDClaims struct {
	Nonce             string `json:"nonce"`
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
}

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
			Email    string `json:"email"`
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
			Email:        req.Email,
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
			token, issueErr = auth.IssueJWT(user.ID, user.Username, user.Email, user.IsAdmin, user.TokenVersion, jwtSecret, jwtTTL())
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
			UserID:       user.ID,
			Username:     user.Username,
			Email:        user.Email,
			IsAdmin:      user.IsAdmin,
			TokenVersion: user.TokenVersion,
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

		token, err := auth.IssueJWT(user.ID, user.Username, user.Email, user.IsAdmin, user.TokenVersion, jwtSecret, jwtTTL())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		setSessionCookie(w, r, token, jwtTTL())

		events.LogSystem(database, "auth", "user.login", "user "+req.Username+" logged in")

		writeSessionResponse(w, &auth.Claims{
			UserID:       user.ID,
			Username:     user.Username,
			Email:        user.Email,
			IsAdmin:      user.IsAdmin,
			TokenVersion: user.TokenVersion,
		}, http.StatusOK)
	}
}

func OIDCProviderLogin(database *gorm.DB, registry *auth.OIDCRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerID := chi.URLParam(r, "provider_id")
		var oidcProvider *auth.OIDCProvider
		if registry != nil && registry.IsReady() {
			oidcProvider = registry.Get(providerID)
		}
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
			ID:         ulid.Make().String(),
			State:      state,
			Nonce:      nonce,
			ProviderID: providerID,
			InviteCode: r.URL.Query().Get("invite_code"),
			ExpiresAt:  time.Now().Add(10 * time.Minute),
			CreatedAt:  time.Now(),
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

func OIDCProviderCallback(database *gorm.DB, registry *auth.OIDCRegistry, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerID := chi.URLParam(r, "provider_id")

		cookie, err := r.Cookie("oidc_state")
		if err != nil {
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: missing state cookie")
			writeError(w, http.StatusBadRequest, "missing state cookie")
			return
		}

		if r.URL.Query().Get("state") != cookie.Value {
			clearOIDCStateCookie(w, r)
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: state mismatch")
			writeError(w, http.StatusBadRequest, "state mismatch")
			return
		}

		var oidcState models.OIDCState
		if err := database.First(&oidcState, "state = ?", cookie.Value).Error; err != nil || oidcState.ExpiresAt.Before(time.Now()) {
			clearOIDCStateCookie(w, r)
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: invalid or expired state")
			writeError(w, http.StatusBadRequest, "invalid or expired state")
			return
		}

		cleanedUp := false
		cleanupState := func() {
			if cleanedUp {
				return
			}
			cleanedUp = true
			_ = database.Delete(&models.OIDCState{}, "id = ?", oidcState.ID).Error
			clearOIDCStateCookie(w, r)
		}

		if oidcState.ProviderID != providerID {
			cleanupState()
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: provider mismatch")
			writeError(w, http.StatusBadRequest, "invalid or expired state")
			return
		}

		var oidcProvider *auth.OIDCProvider
		if registry != nil && registry.IsReady() {
			oidcProvider = registry.Get(providerID)
		}
		if oidcProvider == nil {
			cleanupState()
			writeError(w, http.StatusServiceUnavailable, "OIDC provider unavailable")
			return
		}

		oauth2Token, err := oidcProvider.Config.Exchange(r.Context(), r.URL.Query().Get("code"))
		if err != nil {
			cleanupState()
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: code exchange failed")
			writeError(w, http.StatusUnauthorized, "code exchange failed")
			return
		}

		rawIDToken, ok := oauth2Token.Extra("id_token").(string)
		if !ok {
			cleanupState()
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: missing id_token")
			writeError(w, http.StatusUnauthorized, "missing id_token")
			return
		}

		idToken, err := oidcProvider.Verifier.Verify(r.Context(), rawIDToken)
		if err != nil {
			cleanupState()
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: id_token verification failed")
			writeError(w, http.StatusUnauthorized, "id_token verification failed")
			return
		}

		var idClaims oidcIDClaims
		if err := idToken.Claims(&idClaims); err != nil || idClaims.Nonce != oidcState.Nonce {
			cleanupState()
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: nonce mismatch")
			writeError(w, http.StatusBadRequest, "nonce mismatch")
			return
		}
		issuer := strings.TrimSpace(idToken.Issuer)
		if issuer == "" {
			cleanupState()
			events.LogSystem(database, "auth", "user.login.oidc.failed", "OIDC callback error: missing issuer")
			writeError(w, http.StatusUnauthorized, "invalid id_token issuer")
			return
		}

		var user models.User
		err = database.First(&user, "oidc_issuer = ? AND oidc_subject = ?", issuer, idClaims.Sub).Error
		switch {
		case err == nil:
		case errors.Is(err, gorm.ErrRecordNotFound):
			user, linkExisting, lookupErr := findOIDCUserByEmail(database, issuer, idClaims)
			switch {
			case lookupErr == nil && linkExisting:
				if err := database.Model(&user).Updates(map[string]interface{}{
					"oidc_issuer":  issuer,
					"oidc_subject": idClaims.Sub,
				}).Error; err != nil {
					cleanupState()
					writeError(w, http.StatusInternalServerError, "failed to link account")
					return
				}
				user.OIDCIssuer = issuer
				user.OIDCSubject = idClaims.Sub
			case lookupErr == nil:
				newUser, ok := handleNewOIDCUser(w, database, issuer, idClaims, oidcState, cleanupState)
				if !ok {
					return
				}
				user = newUser
			case errors.Is(lookupErr, errOIDCEmailAmbiguous), errors.Is(lookupErr, errOIDCEmailAlreadyLinked):
				cleanupState()
				writeError(w, http.StatusConflict, lookupErr.Error())
				return
			default:
				cleanupState()
				writeError(w, http.StatusInternalServerError, "failed to lookup user")
				return
			}
		default:
			cleanupState()
			writeError(w, http.StatusInternalServerError, "failed to lookup user")
			return
		}

		token, err := auth.IssueJWT(user.ID, user.Username, user.Email, user.IsAdmin, user.TokenVersion, jwtSecret, jwtTTL())
		if err != nil {
			cleanupState()
			writeError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		cleanupState()
		setSessionCookie(w, r, token, jwtTTL())

		events.LogSystem(database, "auth", "user.login.oidc", "user "+user.Username+" logged in via OIDC")

		http.Redirect(w, r, "/timeline", http.StatusFound)
	}
}

// handleNewOIDCUser applies the OIDC access policy and creates a new user if allowed.
// Returns the created user and true on success, or writes an error response and returns false.
func handleNewOIDCUser(w http.ResponseWriter, database *gorm.DB, issuer string, idClaims oidcIDClaims, oidcState models.OIDCState, cleanupState func()) (models.User, bool) {
	policy, err := getOIDCPolicy(database)
	if err != nil {
		cleanupState()
		writeError(w, http.StatusInternalServerError, "failed to load OIDC policy")
		return models.User{}, false
	}
	switch policy {
	case "existing_only":
		cleanupState()
		writeError(w, http.StatusForbidden, "no account associated with this identity")
		return models.User{}, false
	case "invite_required":
		user := models.User{
			ID:          ulid.Make().String(),
			Username:    preferredOIDCUsername(idClaims),
			Email:       idClaims.Email,
			OIDCIssuer:  issuer,
			OIDCSubject: idClaims.Sub,
			IsAdmin:     false,
			CreatedAt:   time.Now(),
		}
		txErr := database.Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&models.InviteCode{}).
				Where("code = ? AND used_by = '' AND expires_at > ?", oidcState.InviteCode, time.Now()).
				Update("used_by", user.ID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return errInvalidInvite
			}
			return tx.Create(&user).Error
		})
		if txErr == errInvalidInvite {
			cleanupState()
			writeError(w, http.StatusUnauthorized, "invalid or expired invite code")
			return models.User{}, false
		}
		if txErr != nil {
			cleanupState()
			writeError(w, http.StatusInternalServerError, "failed to create user")
			return models.User{}, false
		}
		return user, true
	default:
		user := models.User{
			ID:          ulid.Make().String(),
			Username:    preferredOIDCUsername(idClaims),
			Email:       idClaims.Email,
			OIDCIssuer:  issuer,
			OIDCSubject: idClaims.Sub,
			IsAdmin:     false,
			CreatedAt:   time.Now(),
		}
		if err := database.Create(&user).Error; err != nil {
			cleanupState()
			writeError(w, http.StatusInternalServerError, "failed to create user")
			return models.User{}, false
		}
		return user, true
	}
}

func findOIDCUserByEmail(database *gorm.DB, issuer string, idClaims oidcIDClaims) (models.User, bool, error) {
	if issuer == "" || strings.TrimSpace(idClaims.Email) == "" {
		return models.User{}, false, nil
	}

	var matches []models.User
	if err := database.Where("email = ?", idClaims.Email).Limit(2).Find(&matches).Error; err != nil {
		return models.User{}, false, err
	}
	if len(matches) == 0 {
		return models.User{}, false, nil
	}
	if len(matches) > 1 {
		return models.User{}, false, errOIDCEmailAmbiguous
	}

	user := matches[0]
	if user.OIDCIssuer == "" && user.OIDCSubject == "" {
		return user, true, nil
	}
	if user.OIDCIssuer == issuer && user.OIDCSubject == idClaims.Sub {
		return user, true, nil
	}

	return models.User{}, false, errOIDCEmailAlreadyLinked
}

func preferredOIDCUsername(claims oidcIDClaims) string {
	if claims.PreferredUsername != "" {
		return claims.PreferredUsername
	}
	if claims.Email != "" {
		return claims.Email
	}
	return claims.Sub
}

func clearOIDCStateCookie(w http.ResponseWriter, r *http.Request) {
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
