package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/models"
)

const (
	discordColorConfirmed = 0xED4245
	discordColorSuspected = 0xFEE75C
	discordColorResolved  = 0x57F287
	discordColorTest      = 0x5865F2
)

type DiscordEmbed struct {
	Title     string         `json:"title"`
	Color     int            `json:"color"`
	Fields    []discordField `json:"fields,omitempty"`
	Footer    *discordFooter `json:"footer,omitempty"`
	Timestamp string         `json:"timestamp,omitempty"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordFooter struct {
	Text string `json:"text"`
}

type discordPayload struct {
	Embeds []DiscordEmbed `json:"embeds"`
}

var ExportedSendDiscord = sendDiscord

func BuildDiscordEmbed(inc models.Incident, test bool) DiscordEmbed {
	if test {
		return DiscordEmbed{
			Title:     "Test Notification",
			Color:     discordColorTest,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Footer:    &discordFooter{Text: "Blackbox - Test"},
		}
	}

	embed := DiscordEmbed{
		Title:     inc.Title,
		Color:     discordColorForIncident(inc),
		Timestamp: inc.OpenedAt.UTC().Format(time.RFC3339),
		Fields:    make([]discordField, 0, 3),
		Footer:    &discordFooter{Text: "Blackbox - " + incidentStatusLabel(inc)},
	}

	services := parseIncidentStringList(inc.Services)
	if len(services) > 0 {
		embed.Fields = append(embed.Fields, discordField{
			Name:   "Services",
			Value:  strings.Join(services, ", "),
			Inline: true,
		})
	}

	nodes := parseIncidentStringList(inc.NodeNames)
	if len(nodes) > 0 {
		embed.Fields = append(embed.Fields, discordField{
			Name:   "Nodes",
			Value:  strings.Join(nodes, ", "),
			Inline: true,
		})
	}

	if inc.Status == "resolved" && inc.ResolvedAt != nil {
		embed.Fields = append(embed.Fields, discordField{
			Name:   "Resolved At",
			Value:  inc.ResolvedAt.UTC().Format(time.RFC3339),
			Inline: true,
		})
	}

	return embed
}

func sendDiscord(ctx context.Context, webhookURL string, inc models.Incident, test bool) error {
	payload := discordPayload{Embeds: []DiscordEmbed{BuildDiscordEmbed(inc, test)}}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord request: %w", err)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Printf("notify: close discord response body: %v", errClose)
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("discord returned status %d", resp.StatusCode)
	}

	return nil
}

func discordColorForIncident(inc models.Incident) int {
	switch {
	case inc.Status == "resolved":
		return discordColorResolved
	case inc.Confidence == "suspected":
		return discordColorSuspected
	default:
		return discordColorConfirmed
	}
}

func incidentStatusLabel(inc models.Incident) string {
	if inc.Status == "resolved" {
		return "Resolved"
	}
	if inc.Confidence == "suspected" {
		return "Suspected"
	}
	return "Open"
}

func parseIncidentStringList(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}

	filtered := values[:0]
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		filtered = append(filtered, value)
	}

	return filtered
}
