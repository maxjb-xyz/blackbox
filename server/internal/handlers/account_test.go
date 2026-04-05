package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func updateAccountRequest(claims *auth.Claims, email string) (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPatch, "/api/auth/me", bytes.NewBufferString(`{"email":"`+email+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, claims))
	return req, httptest.NewRecorder()
}

func TestUpdateAccount_RejectsInvalidEmail(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.User{
		ID:       "user-1",
		Username: "alice",
		Email:    "alice@example.com",
	}).Error)

	req, w := updateAccountRequest(&auth.Claims{
		UserID:     "user-1",
		Username:   "alice",
		Email:      "alice@example.com",
		OIDCLinked: false,
	}, "not-an-email")

	handlers.UpdateAccount(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "valid email required", resp["error"])

	var user models.User
	require.NoError(t, database.First(&user, "id = ?", "user-1").Error)
	assert.Equal(t, "alice@example.com", user.Email)
}

func TestUpdateAccount_RejectsOIDCLinkedClaims(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.User{
		ID:       "user-1",
		Username: "alice",
		Email:    "alice@example.com",
	}).Error)

	req, w := updateAccountRequest(&auth.Claims{
		UserID:     "user-1",
		Username:   "alice",
		Email:      "alice@example.com",
		OIDCLinked: true,
	}, "alice+new@example.com")

	handlers.UpdateAccount(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var user models.User
	require.NoError(t, database.First(&user, "id = ?", "user-1").Error)
	assert.Equal(t, "alice@example.com", user.Email)
}

func TestUpdateAccount_RejectsEmailInUse(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.User{
		ID:       "user-1",
		Username: "alice",
		Email:    "alice@example.com",
	}).Error)
	require.NoError(t, database.Create(&models.User{
		ID:       "user-2",
		Username: "bob",
		Email:    "bob@example.com",
	}).Error)

	req, w := updateAccountRequest(&auth.Claims{
		UserID:     "user-1",
		Username:   "alice",
		Email:      "alice@example.com",
		OIDCLinked: false,
	}, "bob@example.com")

	handlers.UpdateAccount(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var user models.User
	require.NoError(t, database.First(&user, "id = ?", "user-1").Error)
	assert.Equal(t, "alice@example.com", user.Email)
}

func TestUpdateAccount_RejectsEmptyEmail(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.User{
		ID:       "user-1",
		Username: "alice",
		Email:    "alice@example.com",
	}).Error)

	req, w := updateAccountRequest(&auth.Claims{
		UserID:     "user-1",
		Username:   "alice",
		Email:      "alice@example.com",
		OIDCLinked: false,
	}, "")

	handlers.UpdateAccount(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "valid email required", resp["error"])

	var user models.User
	require.NoError(t, database.First(&user, "id = ?", "user-1").Error)
	assert.Equal(t, "alice@example.com", user.Email)
}

func TestUpdateAccount_UpdatesEmailAndSession(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.User{
		ID:       "user-1",
		Username: "alice",
		Email:    "alice@example.com",
		IsAdmin:  true,
	}).Error)

	req, w := updateAccountRequest(&auth.Claims{
		UserID:     "user-1",
		Username:   "alice",
		Email:      "alice@example.com",
		OIDCLinked: false,
		IsAdmin:    true,
	}, "alice+new@example.com")

	handlers.UpdateAccount(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		User struct {
			Email      string `json:"email"`
			OIDCLinked bool   `json:"oidc_linked"`
			IsAdmin    bool   `json:"is_admin"`
		} `json:"user"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "alice+new@example.com", resp.User.Email)
	assert.False(t, resp.User.OIDCLinked)
	assert.True(t, resp.User.IsAdmin)

	var user models.User
	require.NoError(t, database.First(&user, "id = ?", "user-1").Error)
	assert.Equal(t, "alice+new@example.com", user.Email)

	claims := sessionClaimsFromResponse(t, w, "jwt-test-secret")
	assert.Equal(t, "alice+new@example.com", claims.Email)
	assert.Equal(t, "user-1", claims.UserID)
	assert.Equal(t, "alice", claims.Username)
	assert.False(t, claims.OIDCLinked)
	assert.True(t, claims.IsAdmin)
}
