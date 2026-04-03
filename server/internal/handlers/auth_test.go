package handlers_test

import (
	"bytes"
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

func TestBootstrap_CreatesAdminAndReturnsToken(t *testing.T) {
	database := newTestDB(t)

	body := `{"username":"admin","password":"SuperSecret1!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.Bootstrap(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotEmpty(t, resp["token"])
	claims, err := auth.VerifyJWT(resp["token"], "jwt-test-secret")
	require.NoError(t, err)
	assert.Equal(t, "admin", claims.Username)

	var user models.User
	require.NoError(t, database.First(&user, "username = ?", "admin").Error)
	assert.True(t, user.IsAdmin)
}

func TestBootstrap_RejectsIfAlreadyBootstrapped(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.User{ID: "01EXISTING", Username: "existing", IsAdmin: true})

	body := `{"username":"hacker","password":"password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.Bootstrap(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestLogin_ValidCredentials(t *testing.T) {
	database := newTestDB(t)
	body := `{"username":"admin","password":"MyPassword1!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	handlers.Bootstrap(database, "jwt-test-secret")(httptest.NewRecorder(), req)

	loginBody := `{"username":"admin","password":"MyPassword1!"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(loginBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()

	handlers.Login(database, "jwt-test-secret")(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(w2.Body).Decode(&resp))
	assert.NotEmpty(t, resp["token"])
	claims, err := auth.VerifyJWT(resp["token"], "jwt-test-secret")
	require.NoError(t, err)
	assert.Equal(t, "admin", claims.Username)
}

func TestLogin_WrongPassword(t *testing.T) {
	database := newTestDB(t)
	body := `{"username":"admin","password":"correct-pass"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	handlers.Bootstrap(database, "jwt-test-secret")(httptest.NewRecorder(), req)

	loginBody := `{"username":"admin","password":"wrong-pass"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(loginBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()

	handlers.Login(database, "jwt-test-secret")(w2, req2)

	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}
