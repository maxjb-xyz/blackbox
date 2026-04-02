package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func adminContext() context.Context {
	return context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{
		UserID:  "01ADMINUSER000000",
		IsAdmin: true,
	})
}

func TestCreateInvite_AdminCreatesCode(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.User{ID: "01ADMINUSER000000", Username: "admin", IsAdmin: true})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/invite", nil)
	req = req.WithContext(adminContext())
	w := httptest.NewRecorder()

	handlers.CreateInvite(database)(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotEmpty(t, resp["code"])
	assert.NotEmpty(t, resp["expires_at"])

	var invite models.InviteCode
	require.NoError(t, database.First(&invite, "code = ?", resp["code"]).Error)
	assert.Equal(t, "01ADMINUSER000000", invite.CreatedBy)
	assert.Empty(t, invite.UsedBy)
}

func TestListInvites_ReturnsUnusedNonExpired(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.InviteCode{ID: "01INVITEID000001", Code: "validcode0000000000000000000000000000000000000000000000000000000a", CreatedBy: "01ADMINUSER000000", ExpiresAt: time.Now().Add(72 * time.Hour), CreatedAt: time.Now()})
	database.Create(&models.InviteCode{ID: "01INVITEID000002", Code: "usedcode00000000000000000000000000000000000000000000000000000000b", CreatedBy: "01ADMINUSER000000", UsedBy: "01SOMEUSER000000", ExpiresAt: time.Now().Add(72 * time.Hour), CreatedAt: time.Now()})
	database.Create(&models.InviteCode{ID: "01INVITEID000003", Code: "expiredcode000000000000000000000000000000000000000000000000000000c", CreatedBy: "01ADMINUSER000000", ExpiresAt: time.Now().Add(-1 * time.Hour), CreatedAt: time.Now()})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/invite", nil)
	req = req.WithContext(adminContext())
	w := httptest.NewRecorder()

	handlers.ListInvites(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp, 1)
}
