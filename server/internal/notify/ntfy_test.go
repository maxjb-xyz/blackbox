package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildNtfyMessage_ConfirmedOpen(t *testing.T) {
	inc := models.Incident{
		Status:     "open",
		Confidence: "confirmed",
		Title:      "nginx down",
		Services:   `["nginx"]`,
		NodeNames:  `["node"]`,
		OpenedAt:   time.Now(),
		Metadata:   "{}",
	}

	title, body, priority, tags := BuildNtfyMessage(inc, EventIncidentOpenedConfirmed, "", false)

	assert.Equal(t, "nginx down", title)
	assert.Equal(t, "urgent", priority)
	assert.Equal(t, "rotating_light", tags)
	assert.Contains(t, body, "nginx")
}

func TestBuildNtfyMessage_SuspectedOpen(t *testing.T) {
	inc := models.Incident{
		Status:     "open",
		Confidence: "suspected",
		Title:      "redis crash",
		Services:   `["redis"]`,
		NodeNames:  `["node"]`,
		OpenedAt:   time.Now(),
		Metadata:   "{}",
	}

	_, _, priority, tags := BuildNtfyMessage(inc, EventIncidentOpenedConfirmed, "", false)

	assert.Equal(t, "high", priority)
	assert.Equal(t, "warning", tags)
}

func TestBuildNtfyMessage_Resolved(t *testing.T) {
	now := time.Now()
	inc := models.Incident{
		Status:     "resolved",
		Confidence: "confirmed",
		Title:      "nginx resolved",
		Services:   `["nginx"]`,
		NodeNames:  `["node"]`,
		OpenedAt:   now.Add(-time.Minute),
		ResolvedAt: &now,
		Metadata:   "{}",
	}

	_, _, priority, tags := BuildNtfyMessage(inc, EventIncidentOpenedConfirmed, "", false)

	assert.Equal(t, "default", priority)
	assert.Equal(t, "white_check_mark", tags)
}

func TestBuildNtfyMessage_Test(t *testing.T) {
	title, _, _, _ := BuildNtfyMessage(models.Incident{}, EventIncidentOpenedConfirmed, "", true)

	assert.Equal(t, "Test Notification", title)
}

func TestBuildNtfyMessage_AIReviewGenerated_WithSummary(t *testing.T) {
	_, body, priority, tags := BuildNtfyMessage(models.Incident{
		ID:         "n3",
		Status:     "open",
		Confidence: "confirmed",
		Title:      "nginx AI review",
		Services:   `["nginx"]`,
		NodeNames:  `["n1"]`,
		OpenedAt:   time.Now(),
		Metadata:   `{"ai_analysis":"Root cause: OOM kill."}`,
	}, EventAIReviewGenerated, "", false)

	assert.Equal(t, "default", priority)
	assert.Equal(t, "robot", tags)
	assert.Contains(t, body, "AI: Root cause: OOM kill.")
}

func TestSendNtfy_SetsHeaders(t *testing.T) {
	var capturedTitle string
	var capturedPriority string
	var capturedTags string
	var capturedBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTitle = r.Header.Get("Title")
		capturedPriority = r.Header.Get("Priority")
		capturedTags = r.Header.Get("Tags")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedBody = string(body)
		assert.Equal(t, "text/plain; charset=utf-8", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	inc := models.Incident{
		Status:     "open",
		Confidence: "confirmed",
		Title:      "nginx down",
		Services:   `["nginx"]`,
		NodeNames:  `["node"]`,
		OpenedAt:   time.Now(),
		Metadata:   "{}",
	}

	err := ExportedSendNtfy(context.Background(), srv.URL, inc, EventIncidentOpenedConfirmed, "", false)
	require.NoError(t, err)

	assert.Equal(t, "nginx down", capturedTitle)
	assert.Equal(t, "urgent", capturedPriority)
	assert.Equal(t, "rotating_light", capturedTags)
	assert.Contains(t, capturedBody, "Services: nginx")
}

func TestSendNtfy_ReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	err := ExportedSendNtfy(context.Background(), srv.URL, models.Incident{
		Services:  `[]`,
		NodeNames: `[]`,
		Metadata:  "{}",
	}, EventIncidentOpenedConfirmed, "", false)

	assert.Error(t, err)
}
