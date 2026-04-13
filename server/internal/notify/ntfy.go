package notify

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"blackbox/server/internal/models"
)

var ExportedSendNtfy = sendNtfy

func BuildNtfyMessage(inc models.Incident, test bool) (title, body, priority, tags string) {
	if test {
		return "Test Notification", "This is a test notification from Blackbox.", "default", "white_check_mark"
	}

	title = inc.Title
	priority, tags = ntfyAttributesForIncident(inc)

	lines := []string{inc.Title}

	services := parseIncidentStringList(inc.Services)
	if len(services) > 0 {
		lines = append(lines, "Services: "+strings.Join(services, ", "))
	}

	nodes := parseIncidentStringList(inc.NodeNames)
	if len(nodes) > 0 {
		lines = append(lines, "Nodes: "+strings.Join(nodes, ", "))
	}

	body = strings.Join(lines, "\n")
	return title, body, priority, tags
}

func sendNtfy(ctx context.Context, topicURL string, inc models.Incident, test bool) error {
	title, body, priority, tags := BuildNtfyMessage(inc, test)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, topicURL, bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("create ntfy request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", title)
	req.Header.Set("Priority", priority)
	req.Header.Set("Tags", tags)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("ntfy returned status %d", resp.StatusCode)
	}

	return nil
}

func ntfyAttributesForIncident(inc models.Incident) (priority, tags string) {
	switch {
	case inc.Status == "resolved":
		return "default", "white_check_mark"
	case inc.Confidence == "suspected":
		return "high", "warning"
	default:
		return "urgent", "rotating_light"
	}
}
