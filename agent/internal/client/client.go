package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"blackbox/shared/types"
)

type PermanentError struct {
	StatusCode int
	Message    string
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("permanent error (status %d): %s", e.StatusCode, e.Message)
}

type Client struct {
	serverURL string
	token     string
	http      *http.Client
}

func New(serverURL, token string) *Client {
	return &Client{
		serverURL: serverURL,
		token:     token,
		http:      &http.Client{},
	}
}

func (c *Client) Send(entry types.Entry) error {
	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.serverURL+"/api/agent/push", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lablog-Agent-Key", c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send entry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return &PermanentError{
				StatusCode: resp.StatusCode,
				Message:    fmt.Sprintf("server returned %d", resp.StatusCode),
			}
		}
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}