package mcp

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
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
	scheme := ""
	host := ""

	if r.URL != nil && r.URL.Scheme != "" {
		scheme = strings.TrimSpace(r.URL.Scheme)
	}
	if scheme == "" {
		switch {
		case r.TLS != nil:
			scheme = "https"
		default:
			scheme = "http"
		}
	}

	host = strings.TrimSpace(r.Host)
	if host == "" && r.URL != nil {
		host = strings.TrimSpace(r.URL.Host)
	}

	if isTrustedProxy(r.RemoteAddr) {
		if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
			scheme = forwardedProto
		}
		if forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
			host = forwardedHost
		}
	}

	return scheme + "://" + host
}

func isTrustedProxy(remoteAddr string) bool {
	host := strings.TrimSpace(remoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	if trusted := os.Getenv("TRUSTED_PROXY_IP"); trusted != "" && host == trusted {
		return true
	}
	parsed := net.ParseIP(host)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback()
}

func normalizeOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("origin must include scheme and host")
	}

	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("origin must include scheme and host")
	}

	port := parsed.Port()
	if port == defaultPortForScheme(scheme) {
		port = ""
	}
	if port != "" {
		return scheme + "://" + net.JoinHostPort(host, port), nil
	}
	if strings.Contains(host, ":") {
		return scheme + "://[" + host + "]", nil
	}
	return scheme + "://" + host, nil
}

func defaultPortForScheme(scheme string) string {
	switch scheme {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}
