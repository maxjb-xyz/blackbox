package mcp

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func ValidateOrigin(r *http.Request, expectedOrigin string) error {
	originHeader := strings.TrimSpace(r.Header.Get("Origin"))
	if originHeader == "" {
		return nil
	}

	actualOrigin, err := normalizeOrigin(originHeader)
	if err != nil {
		return fmt.Errorf("invalid origin header: %w", err)
	}

	allowedOrigin := strings.TrimSpace(expectedOrigin)
	if allowedOrigin == "" {
		allowedOrigin = inferRequestOrigin(r)
	}

	expected, err := normalizeOrigin(allowedOrigin)
	if err != nil {
		return fmt.Errorf("invalid expected origin: %w", err)
	}
	if actualOrigin != expected {
		return fmt.Errorf("origin %q does not match %q", actualOrigin, expected)
	}
	return nil
}

func inferRequestOrigin(r *http.Request) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		switch {
		case r.URL != nil && r.URL.Scheme != "":
			scheme = r.URL.Scheme
		case r.TLS != nil:
			scheme = "https"
		default:
			scheme = "http"
		}
	}

	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" && r.URL != nil {
		host = strings.TrimSpace(r.URL.Host)
	}

	return scheme + "://" + host
}

func normalizeOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("origin must include scheme and host")
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}
