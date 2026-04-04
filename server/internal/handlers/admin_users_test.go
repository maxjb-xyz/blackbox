package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/hub"
	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func adminUserContext(userID string) context.Context {
	return context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{
		UserID:  userID,
		IsAdmin: true,
	})
}

func TestListAdminUsers_ReturnsList(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.User{ID: "u1", Username: "alice", IsAdmin: true})
	database.Create(&models.User{ID: "u2", Username: "bob", IsAdmin: false})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req = req.WithContext(adminUserContext("u1"))
	w := httptest.NewRecorder()

	handlers.ListAdminUsers(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Len(t, result, 2)
	// password hash must not be in response
	for _, u := range result {
		_, hasHash := u["password_hash"]
		assert.False(t, hasHash)
	}
}

func TestUpdateAdminUser_ToggleAdmin(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.User{ID: "admin1", Username: "admin", IsAdmin: true})
	database.Create(&models.User{ID: "user1", Username: "alice", IsAdmin: false})

	body := `{"is_admin": true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/user1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(adminUserContext("admin1"))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "user1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.UpdateAdminUser(database, nil)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var user models.User
	require.NoError(t, database.First(&user, "id = ?", "user1").Error)
	assert.True(t, user.IsAdmin)
	assert.Equal(t, 1, user.TokenVersion)
}

func TestUpdateAdminUser_CannotDemoteSelf(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.User{ID: "admin1", Username: "admin", IsAdmin: true})

	body := `{"is_admin": false}`
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/admin1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(adminUserContext("admin1"))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "admin1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.UpdateAdminUser(database, nil)(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestUpdateAdminUser_RequiresExplicitIsAdmin(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.User{ID: "admin1", Username: "admin", IsAdmin: true}).Error)
	require.NoError(t, database.Create(&models.User{ID: "user1", Username: "alice", IsAdmin: false}).Error)

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/user1", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(adminUserContext("admin1"))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "user1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.UpdateAdminUser(database, nil)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateAdminUser_InvalidatesActiveSessionsOnRoleChange(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.User{ID: "admin1", Username: "admin", IsAdmin: true}).Error)
	require.NoError(t, database.Create(&models.User{ID: "user1", Username: "alice", IsAdmin: false, TokenVersion: 7}).Error)

	eventHub := hub.New()
	_, _, disconnect, unsub, err := eventHub.Subscribe("user1", "10.0.0.1")
	require.NoError(t, err)
	defer unsub()

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/user1", bytes.NewBufferString(`{"is_admin": true}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(adminUserContext("admin1"))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "user1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.UpdateAdminUser(database, eventHub)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	select {
	case reason := <-disconnect:
		assert.Equal(t, "session invalidated", reason)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("role change did not invalidate active sessions")
	}
}

func TestForceLogoutUser_IncrementsTokenVersion(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.User{ID: "admin1", Username: "admin", IsAdmin: true, TokenVersion: 0})
	database.Create(&models.User{ID: "user1", Username: "alice", IsAdmin: false, TokenVersion: 0})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/user1/force-logout", nil)
	req = req.WithContext(adminUserContext("admin1"))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "user1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.ForceLogoutUser(database, nil)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var user models.User
	require.NoError(t, database.First(&user, "id = ?", "user1").Error)
	assert.Equal(t, 1, user.TokenVersion)
}

func TestDeleteAdminUser_DeletesUser(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.User{ID: "admin1", Username: "admin", IsAdmin: true})
	database.Create(&models.User{ID: "user1", Username: "alice", IsAdmin: false})

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/user1", nil)
	req = req.WithContext(adminUserContext("admin1"))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "user1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.DeleteAdminUser(database, nil)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var count int64
	database.Model(&models.User{}).Where("id = ?", "user1").Count(&count)
	assert.EqualValues(t, 0, count)
}

func TestDeleteAdminUser_CannotDeleteSelf(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.User{ID: "admin1", Username: "admin", IsAdmin: true})

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/admin1", nil)
	req = req.WithContext(adminUserContext("admin1"))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "admin1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.DeleteAdminUser(database, nil)(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}
