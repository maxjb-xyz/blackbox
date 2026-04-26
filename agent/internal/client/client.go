package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"blackbox/shared/types"
)

type Client struct {
	serverURL string
	token     string
	nodeName  string
	http      *http.Client
}

type AgentConfig struct {
	FileWatcherEnabled       *bool    `json:"file_watcher_enabled"`
	FileWatcherRedactSecrets bool     `json:"file_watcher_redact_secrets"`
	SystemdUnits             []string `json:"systemd_units"`
}

// PermanentError signals that retrying the request will not help.
type PermanentError struct {
	StatusCode int
	Message    string
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("server returned %d: %s", e.StatusCode, e.Message)
}

func New(serverURL, token, nodeName string) *Client {
	return NewWithHTTPClient(serverURL, token, nodeName, nil)
}

func NewWithHTTPClient(serverURL, token, nodeName string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		serverURL: serverURL,
		token:     token,
		nodeName:  nodeName,
		http:      httpClient,
	}
}

// Send pushes a single entry to the server. Retained for compatibility with
// the existing POST /api/agent/push endpoint; the sender now uses SendBatch.
func (c *Client) Send(ctx context.Context, entry types.Entry) error {
	entry.NodeName = c.nodeName

	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.serverURL+"/api/agent/push", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Blackbox-Agent-Key", c.token)
	req.Header.Set("X-Blackbox-Node-Name", c.nodeName)
	req = req.WithContext(ctx)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send entry: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 256))
		msg := strings.TrimSpace(string(bodyBytes))
		if readErr != nil {
			msg = fmt.Sprintf("(could not read body: %v)", readErr)
		}
		switch resp.StatusCode {
		case 400, 401, 403, 404, 409:
			return &PermanentError{StatusCode: resp.StatusCode, Message: msg}
		}
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, msg)
	}
	return nil
}

// BatchPushError describes a single entry rejected by the server within a batch.
// Permanent is true when the failure is a validation error that will never
// succeed on retry; false means a transient server-side error (e.g. DB failure).
type BatchPushError struct {
	ID        string `json:"id"`
	Reason    string `json:"reason"`
	Permanent bool   `json:"permanent"`
}

type batchPushResponse struct {
	Accepted []string         `json:"accepted"`
	Failed   []BatchPushError `json:"failed"`
}

// SendBatch sends a batch of entries to the server's batch push endpoint.
// On a non-2xx response the whole batch should be retried. On 200, accepted
// and failed lists are returned so the caller can delete accepted rows only.
func (c *Client) SendBatch(ctx context.Context, entries []types.Entry) (accepted []string, failed []BatchPushError, err error) {
	// Copy entries to avoid mutating the caller's slice.
	stamped := make([]types.Entry, len(entries))
	copy(stamped, entries)
	for i := range stamped {
		stamped[i].NodeName = c.nodeName
	}

	body, err := json.Marshal(stamped)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal batch: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.serverURL+"/api/agent/push/batch", bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("create batch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Blackbox-Agent-Key", c.token)
	req.Header.Set("X-Blackbox-Node-Name", c.nodeName)
	req = req.WithContext(ctx)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("send batch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		msg := strings.TrimSpace(string(bodyBytes))
		// 4xx responses are permanent — retrying will not help.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return nil, nil, &PermanentError{StatusCode: resp.StatusCode, Message: msg}
		}
		return nil, nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, msg)
	}

	var result batchPushResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("decode batch response: %w", err)
	}
	return result.Accepted, result.Failed, nil
}

func (c *Client) GetAgentConfig(ctx context.Context, capabilities []string) (AgentConfig, error) {
	// Validate and normalize capability tokens before dispatch so callers can rely on pre-request rejection.
	cleanCapabilities, err := sanitizeCapabilities(capabilities)
	if err != nil {
		return AgentConfig{}, err
	}

	req, err := http.NewRequest(http.MethodGet, c.serverURL+"/api/agent/config", nil)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Blackbox-Agent-Key", c.token)
	req.Header.Set("X-Blackbox-Node-Name", c.nodeName)
	if len(cleanCapabilities) > 0 {
		req.Header.Set("X-Blackbox-Agent-Capabilities", strings.Join(cleanCapabilities, ","))
	}
	req = req.WithContext(ctx)

	resp, err := c.http.Do(req)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("fetch agent config: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 256))
		msg := strings.TrimSpace(string(bodyBytes))
		if readErr != nil {
			msg = fmt.Sprintf("(could not read body: %v)", readErr)
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 &&
			resp.StatusCode != http.StatusRequestTimeout &&
			resp.StatusCode != http.StatusTooManyRequests {
			return AgentConfig{}, &PermanentError{StatusCode: resp.StatusCode, Message: msg}
		}
		return AgentConfig{}, fmt.Errorf("server returned %d: %s", resp.StatusCode, msg)
	}

	var config AgentConfig
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&config); err != nil {
		return AgentConfig{}, fmt.Errorf("decode agent config: %w", err)
	}
	return config, nil
}

func sanitizeCapabilities(capabilities []string) ([]string, error) {
	cleaned := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			continue
		}
		if strings.Contains(capability, ",") {
			return nil, fmt.Errorf("invalid capability %q: capability values must not contain commas", capability)
		}
		cleaned = append(cleaned, capability)
	}
	return cleaned, nil
}
