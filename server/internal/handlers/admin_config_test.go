package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminConfig_ReturnsWebhookSecret(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, &auth.Claims{
		UserID:  "admin1",
		IsAdmin: true,
	}))
	w := httptest.NewRecorder()

	handlers.AdminConfig("my-secret-value")(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "my-secret-value", resp["webhook_secret"])
	assert.Equal(t, "no-store, no-cache, must-revalidate", w.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
	assert.Equal(t, "0", w.Header().Get("Expires"))
}

func TestAdminConfig_EmptyWebhookSecret(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, &auth.Claims{
		UserID:  "admin1",
		IsAdmin: true,
	}))
	w := httptest.NewRecorder()

	handlers.AdminConfig("")(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "", resp["webhook_secret"])
}
