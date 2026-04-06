package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
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

	handlers.AdminConfig(newTestDB(t), "my-secret-value")(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "my-secret-value", resp["webhook_secret"])
	assert.Equal(t, true, resp["file_watcher_redact_secrets"])
	assert.Equal(t, "", resp["ollama_url"])
	assert.Equal(t, "", resp["ollama_model"])
	assert.Equal(t, "analysis", resp["ollama_mode"])
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

	database := newTestDB(t)
	require.NoError(t, database.Create(&models.AppSetting{Key: "file_watcher_redact_secrets", Value: "false"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ollama_url", Value: " http://localhost:11434 "}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ollama_model", Value: " llama3.2 "}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ollama_mode", Value: " enhanced "}).Error)

	handlers.AdminConfig(database, "")(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "", resp["webhook_secret"])
	assert.Equal(t, false, resp["file_watcher_redact_secrets"])
	assert.Equal(t, "http://localhost:11434", resp["ollama_url"])
	assert.Equal(t, "llama3.2", resp["ollama_model"])
	assert.Equal(t, "enhanced", resp["ollama_mode"])
}

func TestUpdateOllamaSettings_RejectsInvalidURL(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/ollama", strings.NewReader(`{"ollama_url":"not a url","ollama_model":"llama3.2"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.UpdateOllamaSettings(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var count int64
	require.NoError(t, database.Model(&models.AppSetting{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateOllamaSettings_PersistsMode(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/ollama", strings.NewReader(`{"ollama_url":"http://localhost:11434","ollama_model":"llama3.2","ollama_mode":"enhanced"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.UpdateOllamaSettings(database)(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	var setting models.AppSetting
	require.NoError(t, database.First(&setting, "key = ?", "ollama_mode").Error)
	assert.Equal(t, "enhanced", setting.Value)
}

func TestAdminConfig_ReturnsOllamaMode(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, &auth.Claims{
		UserID:  "admin1",
		IsAdmin: true,
	}))
	w := httptest.NewRecorder()

	database := newTestDB(t)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ollama_mode", Value: "enhanced"}).Error)

	handlers.AdminConfig(database, "secret")(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "enhanced", resp["ollama_mode"])
}
