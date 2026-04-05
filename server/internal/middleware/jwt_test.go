package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/auth"
	serverdb "blackbox/server/internal/db"
	"blackbox/server/internal/middleware"
	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestJWTAuth_ValidCookie(t *testing.T) {
	secret := "test-secret"
	token, err := auth.IssueJWT("user-1", "alice", "", false, false, 0, secret, time.Hour)
	require.NoError(t, err)

	reached := false
	handler := middleware.JWTAuth(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		claims, ok := auth.ClaimsFromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, "user-1", claims.UserID)
		assert.Equal(t, "alice", claims.Username)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	handler.ServeHTTP(httptest.NewRecorder(), req)
	assert.True(t, reached)
}

func TestJWTAuth_RejectsBearerToken(t *testing.T) {
	secret := "test-secret"
	token, err := auth.IssueJWT("user-1", "alice", "", false, false, 0, secret, time.Hour)
	require.NoError(t, err)

	handler := middleware.JWTAuth(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuth_MissingToken(t *testing.T) {
	handler := middleware.JWTAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	handler := middleware.JWTAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTokenVersionCheck_RejectsStaleToken(t *testing.T) {
	database := func() *gorm.DB {
		db, err := serverdb.Init(":memory:")
		require.NoError(t, err)
		return db
	}()
	// Create user with TokenVersion = 0
	user := models.User{ID: "u1", Username: "alice", TokenVersion: 0}
	require.NoError(t, database.Create(&user).Error)

	// Issue JWT with tv=0
	token, err := auth.IssueJWT("u1", "alice", "", false, false, 0, "secret", time.Hour)
	require.NoError(t, err)

	// Bump user's token_version to 1
	require.NoError(t, database.Model(&user).Update("token_version", 1).Error)

	reached := false
	handler := middleware.JWTAuth("secret")(
		middleware.TokenVersionCheck(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached = true
		})),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, reached)
}

func TestTokenVersionCheck_AcceptsCurrentToken(t *testing.T) {
	database := func() *gorm.DB {
		db, err := serverdb.Init(":memory:")
		require.NoError(t, err)
		return db
	}()
	user := models.User{ID: "u2", Username: "bob", TokenVersion: 3}
	require.NoError(t, database.Create(&user).Error)

	token, err := auth.IssueJWT("u2", "bob", "", false, false, 3, "secret", time.Hour)
	require.NoError(t, err)

	reached := false
	handler := middleware.JWTAuth("secret")(
		middleware.TokenVersionCheck(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached = true
		})),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, reached)
}
