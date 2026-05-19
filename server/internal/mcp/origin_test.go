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

func TestValidateOriginRejectsMismatchedOrigin(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://blackbox.example.com/mcp", nil)
	req.Host = "blackbox.example.com"
	req.Header.Set("Origin", "https://evil.example.com")

	err := ValidateOrigin(req, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "origin")
}
