package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateOriginAllowsMissingOriginAndMatchingOrigin(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://blackbox.example.com/mcp", nil)
	req.Host = "blackbox.example.com"

	require.NoError(t, ValidateOrigin(req, ""))

	req.Header.Set("Origin", "https://blackbox.example.com")
	require.NoError(t, ValidateOrigin(req, ""))
}

func TestValidateOriginAllowsDefaultPortEquivalence(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/mcp", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Host = "127.0.0.1:8080"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "blackbox.example.com:443")
	req.Header.Set("Origin", "https://blackbox.example.com")

	require.NoError(t, ValidateOrigin(req, ""))

	req.Header.Set("Origin", "http://blackbox.example.com")
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("X-Forwarded-Host", "blackbox.example.com:80")
	require.NoError(t, ValidateOrigin(req, ""))
}

func TestValidateOriginIgnoresForwardedHeadersFromUntrustedClients(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/mcp", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Host = "127.0.0.1:8080"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "blackbox.example.com:443")
	req.Header.Set("Origin", "https://blackbox.example.com")

	err := ValidateOrigin(req, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "origin")
}

func TestValidateOriginRejectsMismatchedOrigin(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://blackbox.example.com/mcp", nil)
	req.Host = "blackbox.example.com"
	req.Header.Set("Origin", "https://evil.example.com")

	err := ValidateOrigin(req, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "origin")
}
