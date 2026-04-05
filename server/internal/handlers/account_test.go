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

func TestUpdateAccount_RejectsInvalidEmail(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&models.User{
		ID:       "user-1",
		Username: "alice",
		Email:    "alice@example.com",
	}).Error)

	req := httptest.NewRequest(http.MethodPatch, "/api/auth/me", bytes.NewBufferString(`{"email":"not-an-email"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, &auth.Claims{
		UserID:     "user-1",
		Username:   "alice",
		Email:      "alice@example.com",
		OIDCLinked: false,
	}))
	w := httptest.NewRecorder()

	handlers.UpdateAccount(database, "jwt-test-secret")(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "valid email required", resp["error"])

	var user models.User
	require.NoError(t, database.First(&user, "id = ?", "user-1").Error)
	assert.Equal(t, "alice@example.com", user.Email)
}
