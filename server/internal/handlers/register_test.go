package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister_ValidInvite(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.InviteCode{
		ID:        "01INVITEID000001",
		Code:      "validinvitecode01234567890123456789012345678901234567890123456789",
		CreatedBy: "01ADMINUSER000000",
		ExpiresAt: time.Now().Add(72 * time.Hour),
		CreatedAt: time.Now(),
	})

	body := `{"username":"alice","password":"Hunter2!secure","email":"alice@example.com","invite_code":"validinvitecode01234567890123456789012345678901234567890123456789"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.Register(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.NotNil(t, resp)
	user, ok := resp["user"]
	require.True(t, ok)
	require.NotNil(t, user)
	assert.Equal(t, "alice", user["username"])
	assert.Equal(t, "alice@example.com", user["email"])
	claims := sessionClaimsFromResponse(t, w, "jwt-test-secret")
	assert.Equal(t, "alice", claims.Username)
	assert.Equal(t, "alice@example.com", claims.Email)

	var invite models.InviteCode
	require.NoError(t, database.First(&invite, "code = ?", "validinvitecode01234567890123456789012345678901234567890123456789").Error)
	assert.NotEmpty(t, invite.UsedBy)
}

func TestRegister_InvalidInviteCode(t *testing.T) {
	database := newTestDB(t)

	body := `{"username":"bob","password":"Hunter2!secure","email":"bob@example.com","invite_code":"doesnotexist"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.Register(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRegister_ExpiredInvite(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.InviteCode{
		ID:        "01INVITEID000002",
		Code:      "expiredinvitecode0123456789012345678901234567890123456789012345",
		CreatedBy: "01ADMINUSER000000",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		CreatedAt: time.Now().Add(-73 * time.Hour),
	})

	body := `{"username":"carol","password":"Hunter2!secure","email":"carol@example.com","invite_code":"expiredinvitecode0123456789012345678901234567890123456789012345"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.Register(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRegister_ConcurrentClaimOnlyOneSucceeds(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.InviteCode{
		ID:        "01INVITEID000004",
		Code:      "raceinvitecode0123456789012345678901234567890123456789012345678",
		CreatedBy: "01ADMINUSER000000",
		ExpiresAt: time.Now().Add(72 * time.Hour),
		CreatedAt: time.Now(),
	})

	handler := handlers.Register(database, "jwt-test-secret")

	results := make([]int, 2)
	done := make(chan struct{}, 2)
	for i, username := range []string{"eve", "frank"} {
		i, username := i, username
		go func() {
			body := `{"username":"` + username + `","password":"Hunter2!secure","email":"` + username + `@example.com","invite_code":"raceinvitecode0123456789012345678901234567890123456789012345678"}`
			req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler(w, req)
			results[i] = w.Code
			done <- struct{}{}
		}()
	}
	<-done
	<-done

	codes := []int{results[0], results[1]}
	created := 0
	for _, c := range codes {
		if c == http.StatusCreated {
			created++
		}
	}
	assert.Equal(t, 1, created, "exactly one registration should succeed")

	var invite models.InviteCode
	require.NoError(t, database.First(&invite, "code = ?", "raceinvitecode0123456789012345678901234567890123456789012345678").Error)
	assert.NotEmpty(t, invite.UsedBy, "invite must be claimed by exactly one user")
}

func TestRegister_AlreadyUsedInvite(t *testing.T) {
	database := newTestDB(t)
	database.Create(&models.InviteCode{
		ID:        "01INVITEID000003",
		Code:      "usedinvitecode012345678901234567890123456789012345678901234567890",
		CreatedBy: "01ADMINUSER000000",
		UsedBy:    "01OTHERUSERID000",
		ExpiresAt: time.Now().Add(72 * time.Hour),
		CreatedAt: time.Now(),
	})

	body := `{"username":"dave","password":"Hunter2!secure","email":"dave@example.com","invite_code":"usedinvitecode012345678901234567890123456789012345678901234567890"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.Register(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRegister_RequiresEmail(t *testing.T) {
	database := newTestDB(t)

	body := `{"username":"erin","password":"Hunter2!secure","invite_code":"doesnotmatter"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.Register(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
