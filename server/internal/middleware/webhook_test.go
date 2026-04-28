package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookAuth_ValidSecret(t *testing.T) {
	reached := false
	handler := middleware.WebhookAuth("my-secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/uptime", nil)
	req.Header.Set("X-Webhook-Secret", "my-secret")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	assert.True(t, reached)
}

func TestWebhookAuth_WrongSecret(t *testing.T) {
	handler := middleware.WebhookAuth("my-secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/uptime", nil)
	req.Header.Set("X-Webhook-Secret", "wrong-secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWebhookAuth_MissingHeader(t *testing.T) {
	handler := middleware.WebhookAuth("my-secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/uptime", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWebhookAuth_EmptyConfiguredSecret(t *testing.T) {
	handler := middleware.WebhookAuth("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/uptime", nil)
	req.Header.Set("X-Webhook-Secret", "anything")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWebhookAuthFuncMulti_ValidSecret(t *testing.T) {
	var injectedID string
	handler := middleware.WebhookAuthFuncMulti(func() map[string]string {
		return map[string]string{"src-1": "secret-a", "src-2": "secret-b"}
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := middleware.WebhookSourceIDFromContext(r.Context())
		require.True(t, ok)
		injectedID = id
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/komodo", nil)
	req.Header.Set("X-Webhook-Secret", "secret-b")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	assert.Equal(t, "src-2", injectedID)
}

func TestWebhookAuthFuncMulti_WrongSecret(t *testing.T) {
	handler := middleware.WebhookAuthFuncMulti(func() map[string]string {
		return map[string]string{"src-1": "secret-a"}
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/komodo", nil)
	req.Header.Set("X-Webhook-Secret", "wrong")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWebhookAuthFuncMulti_EmptySecrets(t *testing.T) {
	handler := middleware.WebhookAuthFuncMulti(func() map[string]string {
		return map[string]string{}
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/komodo", nil)
	req.Header.Set("X-Webhook-Secret", "anything")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWebhookAuthFuncMulti_NoHeader(t *testing.T) {
	handler := middleware.WebhookAuthFuncMulti(func() map[string]string {
		return map[string]string{"src-1": "secret-a"}
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/komodo", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
