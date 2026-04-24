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

	handlers.AdminConfig(newTestDB(t), "my-secret-value", nil)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "my-secret-value", resp["webhook_secret"])
	assert.Equal(t, true, resp["file_watcher_redact_secrets"])
	assert.Equal(t, "ollama", resp["ai_provider"])
	assert.Equal(t, "", resp["ai_url"])
	assert.Equal(t, "", resp["ai_model"])
	assert.Equal(t, false, resp["ai_api_key_set"])
	assert.Equal(t, "analysis", resp["ai_mode"])
	assert.Equal(t, "no-store, no-cache, must-revalidate", w.Header().Get("Cache-Control"))
}

func TestAdminConfig_ReturnsAISettings(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, &auth.Claims{
		UserID:  "admin1",
		IsAdmin: true,
	}))
	w := httptest.NewRecorder()

	database := newTestDB(t)
	require.NoError(t, database.Create(&models.AppSetting{Key: "file_watcher_redact_secrets", Value: "false"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ai_provider", Value: "openai_compat"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ai_url", Value: " https://api.openai.com "}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ai_model", Value: " gpt-4o-mini "}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ai_api_key", Value: "sk-secret"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ai_mode", Value: " enhanced "}).Error)

	handlers.AdminConfig(database, "", nil)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "openai_compat", resp["ai_provider"])
	assert.Equal(t, "https://api.openai.com", resp["ai_url"])
	assert.Equal(t, "gpt-4o-mini", resp["ai_model"])
	assert.Equal(t, true, resp["ai_api_key_set"])
	assert.Equal(t, "enhanced", resp["ai_mode"])
}

func TestAdminConfig_LegacyOllamaFallback(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, &auth.Claims{
		UserID:  "admin1",
		IsAdmin: true,
	}))
	w := httptest.NewRecorder()

	database := newTestDB(t)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ollama_url", Value: "http://localhost:11434"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ollama_model", Value: "llama3.2"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ollama_mode", Value: "enhanced"}).Error)

	handlers.AdminConfig(database, "", nil)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "ollama", resp["ai_provider"])
	assert.Equal(t, "http://localhost:11434", resp["ai_url"])
	assert.Equal(t, "llama3.2", resp["ai_model"])
	assert.Equal(t, false, resp["ai_api_key_set"])
	assert.Equal(t, "enhanced", resp["ai_mode"])
}

func TestUpdateAISettings_RejectsInvalidURL(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/ai", strings.NewReader(`{"ai_url":"not a url","ai_model":"llama3.2"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.UpdateAISettings(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var count int64
	require.NoError(t, database.Model(&models.AppSetting{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateAISettings_RejectsInvalidProvider(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/ai", strings.NewReader(`{"ai_provider":"anthropic","ai_url":"http://localhost","ai_model":"m"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.UpdateAISettings(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var count int64
	require.NoError(t, database.Model(&models.AppSetting{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateAISettings_PersistsMode(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/ai", strings.NewReader(`{"ai_provider":"ollama","ai_url":"http://localhost:11434","ai_model":"llama3.2","ai_mode":"enhanced"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.UpdateAISettings(database)(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	var setting models.AppSetting
	require.NoError(t, database.First(&setting, "key = ?", "ai_mode").Error)
	assert.Equal(t, "enhanced", setting.Value)
}

func TestUpdateAISettings_PreservesAPIKeyWhenBlank(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	// Initial save with API key
	body1 := `{"ai_provider":"openai_compat","ai_url":"https://api.openai.com","ai_model":"gpt-4o","ai_api_key":"sk-original","ai_mode":"analysis"}`
	req1 := httptest.NewRequest(http.MethodPut, "/api/admin/settings/ai", strings.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	handlers.UpdateAISettings(database)(w1, req1)
	assert.Equal(t, http.StatusNoContent, w1.Code)

	// Update without sending API key
	body2 := `{"ai_provider":"openai_compat","ai_url":"https://api.openai.com","ai_model":"gpt-4o-mini","ai_api_key":"","ai_mode":"analysis"}`
	req2 := httptest.NewRequest(http.MethodPut, "/api/admin/settings/ai", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	handlers.UpdateAISettings(database)(w2, req2)

	assert.Equal(t, http.StatusNoContent, w2.Code)
	var keySetting models.AppSetting
	require.NoError(t, database.First(&keySetting, "key = ?", "ai_api_key").Error)
	assert.Equal(t, "sk-original", keySetting.Value)
	var modelSetting models.AppSetting
	require.NoError(t, database.First(&modelSetting, "key = ?", "ai_model").Error)
	assert.Equal(t, "gpt-4o-mini", modelSetting.Value)
}

func TestTestAISettings_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/generate", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"OK"}`))
	}))
	t.Cleanup(srv.Close)

	database := newTestDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings/ai/test", strings.NewReader(`{"ai_provider":"ollama","ai_url":"`+srv.URL+`","ai_model":"llama3.2"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.TestAISettings(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, true, resp["ok"])
	assert.Equal(t, "OK", resp["response"])
}

func TestTestAISettings_UsesStoredAPIKeyWhenBlank(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer sk-saved", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	t.Cleanup(srv.Close)

	database := newTestDB(t)
	require.NoError(t, database.Create(&models.AppSetting{Key: "ai_api_key", Value: "sk-saved"}).Error)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings/ai/test", strings.NewReader(`{"ai_provider":"openai_compat","ai_url":"`+srv.URL+`","ai_model":"gpt-4o-mini","ai_api_key":"","ai_mode":"analysis"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.TestAISettings(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, true, resp["ok"])
	assert.Equal(t, "OK", resp["response"])
}

// --- MCP Settings Tests ---

func TestUpdateMCPSettings_EnableGeneratesToken(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	body := `{"mcp_enabled":true,"mcp_port":13001}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.UpdateMCPSettings(database, nil)(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	var tokenSetting models.AppSetting
	require.NoError(t, database.First(&tokenSetting, "key = ?", "mcp_auth_token").Error)
	assert.NotEmpty(t, tokenSetting.Value)

	var enabledSetting models.AppSetting
	require.NoError(t, database.First(&enabledSetting, "key = ?", "mcp_enabled").Error)
	assert.Equal(t, "true", enabledSetting.Value)
}

func TestUpdateMCPSettings_DisablePersists(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	body := `{"mcp_enabled":false,"mcp_port":13002}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.UpdateMCPSettings(database, nil)(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	var enabledSetting models.AppSetting
	require.NoError(t, database.First(&enabledSetting, "key = ?", "mcp_enabled").Error)
	assert.Equal(t, "false", enabledSetting.Value)
}

func TestUpdateMCPSettings_RejectsInvalidPort(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	body := `{"mcp_enabled":true,"mcp_port":80}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.UpdateMCPSettings(database, nil)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateMCPSettings_PreservesExistingToken(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	require.NoError(t, database.Create(&models.AppSetting{Key: "mcp_auth_token", Value: "existing-token-value-abcdef1234"}).Error)

	body := `{"mcp_enabled":true,"mcp_port":13003}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.UpdateMCPSettings(database, nil)(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	var tokenSetting models.AppSetting
	require.NoError(t, database.First(&tokenSetting, "key = ?", "mcp_auth_token").Error)
	assert.Equal(t, "existing-token-value-abcdef1234", tokenSetting.Value)
}

func TestRegenerateMCPToken_ReturnsNewSuffix(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	// Seed initial token and enabled state
	require.NoError(t, database.Create(&models.AppSetting{Key: "mcp_auth_token", Value: "old-token-12345678"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "mcp_enabled", Value: "false"}).Error)
	require.NoError(t, database.Create(&models.AppSetting{Key: "mcp_port", Value: "13004"}).Error)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/settings/mcp/regenerate-token", nil)
	w := httptest.NewRecorder()

	handlers.RegenerateMCPToken(database, nil)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	suffix, ok := resp["mcp_auth_token_suffix"]
	require.True(t, ok, "response should have mcp_auth_token_suffix")
	assert.Len(t, suffix, 8)

	var newTokenSetting models.AppSetting
	require.NoError(t, database.First(&newTokenSetting, "key = ?", "mcp_auth_token").Error)
	assert.NotEqual(t, "old-token-12345678", newTokenSetting.Value)
	assert.True(t, strings.HasSuffix(newTokenSetting.Value, suffix))
}
