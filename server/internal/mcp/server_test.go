package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPManagerMountedHandlerReturns503WhenDisabled(t *testing.T) {
	t.Parallel()

	manager := NewMCPManager(nil)
	require.NoError(t, manager.ApplySettings(false, ""))

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	manager.Handler().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.JSONEq(t, `{"error":"mcp is disabled"}`, rr.Body.String())
}

func TestMCPManagerMountedHandlerRequiresBearerToken(t *testing.T) {
	t.Parallel()

	manager := NewMCPManager(nil)
	require.NoError(t, manager.ApplySettings(true, "secret"))

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	manager.Handler().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestMCPManagerMountedHandlerPassesAuthorizedRequestToUnderlyingHandler(t *testing.T) {
	t.Parallel()

	manager := NewMCPManager(nil)
	require.NoError(t, manager.ApplySettings(true, "secret"))

	nextCalled := false
	manager.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "https://blackbox.example.com/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Origin", "https://blackbox.example.com:443")
	rr := httptest.NewRecorder()

	manager.Handler().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.True(t, nextCalled, "expected underlying handler to be called")
}
