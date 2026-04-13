package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/models"
)

type SlackPayload struct {
	Attachments []SlackAttachment `json:"attachments"`
}

type SlackAttachment struct {
	Fallback string       `json:"fallback"`
	Color    string       `json:"color"`
	Title    string       `json:"title"`
	Fields   []slackField `json:"fields,omitempty"`
	Footer   string       `json:"footer,omitempty"`
	Ts       int64        `json:"ts,omitempty"`
}

type slackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

var ExportedSendSlack = sendSlack

func BuildSlackPayload(inc models.Incident, test bool) SlackPayload {
	if test {
		return SlackPayload{
			Attachments: []SlackAttachment{{
				Fallback: "Test notification from Blackbox",
				Color:    "#5865F2",
				Title:    "Test Notification",
				Footer:   "Blackbox",
				Ts:       time.Now().Unix(),
			}},
		}
	}

	attachment := SlackAttachment{
		Fallback: inc.Title,
		Color:    slackColorForIncident(inc),
		Title:    inc.Title,
		Footer:   "Blackbox",
		Ts:       inc.OpenedAt.Unix(),
		Fields:   make([]slackField, 0, 2),
	}

	services := parseIncidentStringList(inc.Services)
	if len(services) > 0 {
		attachment.Fields = append(attachment.Fields, slackField{
			Title: "Services",
			Value: strings.Join(services, ", "),
			Short: true,
		})
	}

	nodes := parseIncidentStringList(inc.NodeNames)
	if len(nodes) > 0 {
		attachment.Fields = append(attachment.Fields, slackField{
			Title: "Nodes",
			Value: strings.Join(nodes, ", "),
			Short: true,
		})
	}

	return SlackPayload{Attachments: []SlackAttachment{attachment}}
}

func sendSlack(ctx context.Context, webhookURL string, inc models.Incident, test bool) error {
	payload := BuildSlackPayload(inc, test)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	return nil
}

func slackColorForIncident(inc models.Incident) string {
	switch {
	case inc.Status == "resolved":
		return "#57F287"
	case inc.Confidence == "suspected":
		return "#FEE75C"
	default:
		return "#ED4245"
	}
}
