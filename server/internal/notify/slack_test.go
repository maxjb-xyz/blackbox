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

func TestBuildSlackPayload_ConfirmedOpen(t *testing.T) {
	inc := models.Incident{
		ID:         "s1",
		Status:     "open",
		Confidence: "confirmed",
		Title:      "nginx down",
		Services:   `["nginx"]`,
		NodeNames:  `["node1"]`,
		OpenedAt:   time.Now(),
		Metadata:   "{}",
	}

	payload := BuildSlackPayload(inc, false)

	require.Len(t, payload.Attachments, 1)
	assert.Equal(t, "#ED4245", payload.Attachments[0].Color)
	assert.Equal(t, "nginx down", payload.Attachments[0].Title)
}

func TestBuildSlackPayload_Resolved(t *testing.T) {
	now := time.Now()
	inc := models.Incident{
		ID:         "s2",
		Status:     "resolved",
		Confidence: "confirmed",
		Title:      "nginx resolved",
		Services:   `["nginx"]`,
		NodeNames:  `["node1"]`,
		OpenedAt:   now.Add(-3 * time.Minute),
		ResolvedAt: &now,
		Metadata:   "{}",
	}

	payload := BuildSlackPayload(inc, false)

	assert.Equal(t, "#57F287", payload.Attachments[0].Color)
}

func TestBuildSlackPayload_Test(t *testing.T) {
	payload := BuildSlackPayload(models.Incident{}, true)

	require.Len(t, payload.Attachments, 1)
	assert.Equal(t, "#5865F2", payload.Attachments[0].Color)
}

func TestSendSlack_PostsPayload(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		body, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := ExportedSendSlack(context.Background(), srv.URL, models.Incident{
		Status:     "open",
		Confidence: "confirmed",
		Title:      "t",
		Services:   `["svc"]`,
		NodeNames:  `["node"]`,
		OpenedAt:   time.Now(),
		Metadata:   "{}",
	}, false)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	_, ok := payload["attachments"]
	assert.True(t, ok)
}

func TestSendSlack_ReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	err := ExportedSendSlack(context.Background(), srv.URL, models.Incident{
		Services:  `[]`,
		NodeNames: `[]`,
		Metadata:  "{}",
	}, false)

	assert.Error(t, err)
}
