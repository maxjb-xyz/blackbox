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

func (c *Client) Send(ctx context.Context, entry types.Entry) error {
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
	defer resp.Body.Close()

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
