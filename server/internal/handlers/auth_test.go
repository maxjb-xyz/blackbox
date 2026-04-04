package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sessionClaimsFromResponse(t *testing.T, w *httptest.ResponseRecorder, secret string) *auth.Claims {
	t.Helper()

	var sessionCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == auth.SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	require.NotNil(t, sessionCookie)

	claims, err := auth.VerifyJWT(sessionCookie.Value, secret)
	require.NoError(t, err)
	return claims
}

func TestBootstrap_CreatesAdminAndSetsSessionCookie(t *testing.T) {
	database := newTestDB(t)

	body := `{"username":"admin","password":"SuperSecret1!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.Bootstrap(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	claims := sessionClaimsFromResponse(t, w, "jwt-test-secret")
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

func TestBootstrap_ConcurrentRequestsOnlyOneSucceeds(t *testing.T) {
	database := newTestDB(t)
	handler := handlers.Bootstrap(database, "jwt-test-secret")

	results := make([]int, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i, username := range []string{"admin-a", "admin-b"} {
		i, username := i, username
		go func() {
			defer wg.Done()
			body := `{"username":"` + username + `","password":"SuperSecret1!"}`
			req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler(w, req)
			results[i] = w.Code
		}()
	}
	wg.Wait()

	created := 0
	for _, code := range results {
		if code == http.StatusCreated {
			created++
		}
	}
	assert.Equal(t, 1, created)

	var count int64
	require.NoError(t, database.Model(&models.User{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
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
	var resp map[string]map[string]interface{}
	require.NoError(t, json.NewDecoder(w2.Body).Decode(&resp))
	claims := sessionClaimsFromResponse(t, w2, "jwt-test-secret")
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

func TestCurrentUser_ReturnsSessionUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, &auth.Claims{
		UserID:   "user-1",
		Username: "alice",
		IsAdmin:  true,
	}))
	w := httptest.NewRecorder()

	handlers.CurrentUser()(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		User struct {
			Username string `json:"username"`
			IsAdmin  bool   `json:"is_admin"`
		} `json:"user"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "alice", resp.User.Username)
	assert.True(t, resp.User.IsAdmin)
}

func TestLogout_ClearsSessionCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	w := httptest.NewRecorder()

	handlers.Logout()(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var sessionCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == auth.SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	require.NotNil(t, sessionCookie)
	assert.Equal(t, -1, sessionCookie.MaxAge)
}
