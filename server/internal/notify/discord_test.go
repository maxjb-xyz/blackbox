package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDiscordEmbed_ConfirmedOpen(t *testing.T) {
	inc := models.Incident{
		ID:         "i1",
		Status:     "open",
		Confidence: "confirmed",
		Title:      "nginx - monitor down",
		Services:   `["nginx"]`,
		NodeNames:  `["server1"]`,
		OpenedAt:   time.Now(),
		Metadata:   "{}",
	}

	embed := BuildDiscordEmbed(inc, false)

	assert.Equal(t, discordColorConfirmed, embed.Color)
	assert.Equal(t, "nginx - monitor down", embed.Title)
	assert.NotEmpty(t, embed.Fields)
}

func TestBuildDiscordEmbed_SuspectedOpen(t *testing.T) {
	inc := models.Incident{
		ID:         "i2",
		Status:     "open",
		Confidence: "suspected",
		Title:      "redis - crash",
		Services:   `["redis"]`,
		NodeNames:  `["node1"]`,
		OpenedAt:   time.Now(),
		Metadata:   "{}",
	}

	embed := BuildDiscordEmbed(inc, false)

	assert.Equal(t, discordColorSuspected, embed.Color)
}

func TestBuildDiscordEmbed_Resolved(t *testing.T) {
	now := time.Now()
	inc := models.Incident{
		ID:         "i3",
		Status:     "resolved",
		Confidence: "confirmed",
		Title:      "nginx - resolved",
		Services:   `["nginx"]`,
		NodeNames:  `["server1"]`,
		OpenedAt:   now.Add(-5 * time.Minute),
		ResolvedAt: &now,
		Metadata:   "{}",
	}

	embed := BuildDiscordEmbed(inc, false)

	assert.Equal(t, discordColorResolved, embed.Color)
}

func TestBuildDiscordEmbed_Test(t *testing.T) {
	embed := BuildDiscordEmbed(models.Incident{}, true)

	assert.Equal(t, discordColorTest, embed.Color)
	assert.Equal(t, "Test Notification", embed.Title)
}

func TestSendDiscord_PostsPayload(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		body, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	inc := models.Incident{
		ID:         "i4",
		Status:     "open",
		Confidence: "confirmed",
		Title:      "test incident",
		Services:   `["svc"]`,
		NodeNames:  `["node"]`,
		OpenedAt:   time.Now(),
		Metadata:   "{}",
	}

	err := ExportedSendDiscord(context.Background(), srv.URL, inc, false)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	embeds, ok := payload["embeds"].([]any)
	require.True(t, ok)
	assert.Len(t, embeds, 1)
}

func TestSendDiscord_ReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := ExportedSendDiscord(context.Background(), srv.URL, models.Incident{
		Services:  `[]`,
		NodeNames: `[]`,
		Metadata:  "{}",
	}, false)

	assert.Error(t, err)
}
