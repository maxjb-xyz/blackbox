package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentAuth_ValidToken(t *testing.T) {
	config, err := middleware.NewAgentAuthConfig("node-a=secret-token")
	require.NoError(t, err)
	reached := false
	handler := middleware.AgentAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		nodeName, ok := middleware.AgentNodeFromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, "node-a", nodeName)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", nil)
	req.Header.Set("X-Blackbox-Agent-Key", "secret-token")
	req.Header.Set("X-Blackbox-Node-Name", "node-a")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	assert.True(t, reached)
}

func TestAgentAuth_WrongToken(t *testing.T) {
	config, err := middleware.NewAgentAuthConfig("node-a=secret-token")
	require.NoError(t, err)
	handler := middleware.AgentAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", nil)
	req.Header.Set("X-Blackbox-Agent-Key", "wrong-token")
	req.Header.Set("X-Blackbox-Node-Name", "node-a")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAgentAuth_MissingNodeHeader(t *testing.T) {
	config, err := middleware.NewAgentAuthConfig("node-a=secret-token")
	require.NoError(t, err)
	handler := middleware.AgentAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/agent/push", nil)
	req.Header.Set("X-Blackbox-Agent-Key", "secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
