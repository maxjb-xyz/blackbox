// Package proxmox implements a read-only event source that polls the
// Proxmox VE cluster tasks API and emits blackbox entries for started,
// completed, and failed tasks (VM lifecycle, backups, migrations, etc.).
package proxmox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	tasksPath      = "/api2/json/cluster/tasks"
	requestTimeout = 10 * time.Second
	maxBodyBytes   = 2 << 20 // 2 MiB - PVE task lists are small
)

// Task mirrors a single entry from /api2/json/cluster/tasks.
// Only the fields we use are declared; PVE returns more.
type Task struct {
	UPID      string `json:"upid"`
	Node      string `json:"node"`
	PID       int    `json:"pid"`
	StartTime int64  `json:"starttime"`
	EndTime   int64  `json:"endtime,omitempty"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	User      string `json:"user"`
	Status    string `json:"status,omitempty"` // "OK", "ERROR: ...", absent while running
}

// Client is a minimal PVE API client for the tasks endpoint. It is safe
// for concurrent use.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Config controls how the Client talks to Proxmox.
type Config struct {
	// BaseURL is the Proxmox API root, e.g. "https://pve01.example.com:8006".
	BaseURL string
	// APIToken is the full PVE API token string of the form
	// "user@realm!tokenid=uuid". See
	// https://pve.proxmox.com/wiki/Proxmox_VE_API#API_Tokens.
	APIToken string
	// InsecureSkipVerify disables TLS certificate verification. Needed for
	// self-signed homelab setups; leave false in production.
	InsecureSkipVerify bool
	// HTTPClient lets tests inject a custom transport.
	HTTPClient *http.Client
}

// New builds a Client. BaseURL and APIToken must be non-empty.
func New(cfg Config) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("proxmox: BaseURL is required")
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("proxmox: invalid BaseURL: %w", err)
	}
	token := strings.TrimSpace(cfg.APIToken)
	if token == "" {
		return nil, fmt.Errorf("proxmox: APIToken is required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		transport := &http.Transport{
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify}, //nolint:gosec // opt-in for homelab self-signed
			ResponseHeaderTimeout: requestTimeout,
			IdleConnTimeout:       60 * time.Second,
		}
		httpClient = &http.Client{
			Timeout:   requestTimeout,
			Transport: transport,
		}
	}

	return &Client{
		baseURL: base,
		token:   token,
		http:    httpClient,
	}, nil
}

// ListTasks fetches the current cluster task list. The result is
// ordered newest-first by the PVE API.
func (c *Client) ListTasks(ctx context.Context) ([]Task, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+tasksPath, nil)
	if err != nil {
		return nil, fmt.Errorf("proxmox: build request: %w", err)
	}
	req.Header.Set("Authorization", "PVEAPIToken="+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("proxmox: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("proxmox: authentication failed (HTTP %d) - check PROXMOX_API_TOKEN", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("proxmox: unexpected HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Data []Task `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("proxmox: decode tasks: %w", err)
	}
	return payload.Data, nil
}
