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
	return &Client{
		serverURL: serverURL,
		token:     token,
		nodeName:  nodeName,
		http:      &http.Client{Timeout: 10 * time.Second},
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
type BatchPushError struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type batchPushResponse struct {
	Accepted []string         `json:"accepted"`
	Failed   []BatchPushError `json:"failed"`
}

// SendBatch sends a batch of entries to the server's batch push endpoint.
// On a non-2xx response the whole batch should be retried. On 200, accepted
// and failed lists are returned so the caller can delete accepted rows only.
func (c *Client) SendBatch(ctx context.Context, entries []types.Entry) (accepted []string, failed []BatchPushError, err error) {
	for i := range entries {
		entries[i].NodeName = c.nodeName
	}

	body, err := json.Marshal(entries)
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
		return nil, nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var result batchPushResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("decode batch response: %w", err)
	}
	return result.Accepted, result.Failed, nil
}

func (c *Client) GetAgentConfig(ctx context.Context) (AgentConfig, error) {
	req, err := http.NewRequest(http.MethodGet, c.serverURL+"/api/agent/config", nil)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Blackbox-Agent-Key", c.token)
	req.Header.Set("X-Blackbox-Node-Name", c.nodeName)
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
		return AgentConfig{}, fmt.Errorf("server returned %d: %s", resp.StatusCode, msg)
	}

	var config AgentConfig
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&config); err != nil {
		return AgentConfig{}, fmt.Errorf("decode agent config: %w", err)
	}
	return config, nil
}
