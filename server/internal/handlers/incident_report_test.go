package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeResolvedIncident(id string, opened, resolved time.Time) models.Incident {
	return models.Incident{
		ID:         id,
		OpenedAt:   opened,
		ResolvedAt: &resolved,
		Status:     "resolved",
		Confidence: "confirmed",
		Title:      "nginx container died on node-01",
		Services:   `["nginx","postgres"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{"ai_analysis":"Root cause: OOM kill triggered by a memory spike in nginx.","ai_model":"llama3"}`,
	}
}

func TestDownloadIncidentReport_NotFound(t *testing.T) {
	database := newTestDB(t)

	r := chi.NewRouter()
	r.Get("/api/incidents/{id}/report.pdf", handlers.DownloadIncidentReport(database))

	req := httptest.NewRequest(http.MethodGet, "/api/incidents/nonexistent/report.pdf", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestDownloadIncidentReport_NotResolved(t *testing.T) {
	database := newTestDB(t)

	inc := models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   time.Now().UTC(),
		Status:     "open",
		Confidence: "confirmed",
		Title:      "open incident",
		Services:   `["nginx"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{}`,
	}
	require.NoError(t, database.Create(&inc).Error)

	r := chi.NewRouter()
	r.Get("/api/incidents/{id}/report.pdf", handlers.DownloadIncidentReport(database))

	req := httptest.NewRequest(http.MethodGet, "/api/incidents/"+inc.ID+"/report.pdf", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "incident is not resolved")
}

func TestDownloadIncidentReport_Success(t *testing.T) {
	database := newTestDB(t)

	opened := time.Now().UTC().Add(-2 * time.Hour)
	resolved := time.Now().UTC()
	incID := ulid.Make().String()
	inc := makeResolvedIncident(incID, opened, resolved)
	require.NoError(t, database.Create(&inc).Error)

	entry1 := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: opened.Add(time.Minute),
		Source:    "agent",
		NodeName:  "node-01",
		Service:   "nginx",
		Event:     "container.die",
		Content:   "Exit code 137",
		Metadata:  `{}`,
	}
	entry2 := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: opened,
		Source:    "agent",
		NodeName:  "node-01",
		Service:   "nginx",
		Event:     "oom_kill",
		Content:   "",
		Metadata:  `{}`,
	}
	require.NoError(t, database.Create(&entry1).Error)
	require.NoError(t, database.Create(&entry2).Error)

	require.NoError(t, database.Create(&models.IncidentEntry{
		IncidentID: incID,
		EntryID:    entry1.ID,
		Role:       "trigger",
		Score:      0,
		Reason:     "",
	}).Error)
	require.NoError(t, database.Create(&models.IncidentEntry{
		IncidentID: incID,
		EntryID:    entry2.ID,
		Role:       "cause",
		Score:      85,
		Reason:     "",
	}).Error)

	r := chi.NewRouter()
	r.Get("/api/incidents/{id}/report.pdf", handlers.DownloadIncidentReport(database))

	req := httptest.NewRequest(http.MethodGet, "/api/incidents/"+incID+"/report.pdf", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/pdf", rr.Header().Get("Content-Type"))
	assert.Contains(t, rr.Header().Get("Content-Disposition"), "incident-"+incID+"-report.pdf")
	body := rr.Body.Bytes()
	assert.NotEmpty(t, body)
	assert.True(t, strings.HasPrefix(string(body), "%PDF-"), "body must start with PDF magic bytes")
}

func TestDownloadIncidentReport_NoAIAnalysis(t *testing.T) {
	database := newTestDB(t)

	opened := time.Now().UTC().Add(-30 * time.Minute)
	resolved := time.Now().UTC()
	incID := ulid.Make().String()
	inc := models.Incident{
		ID:         incID,
		OpenedAt:   opened,
		ResolvedAt: &resolved,
		Status:     "resolved",
		Confidence: "suspected",
		Title:      "redis timeout",
		Services:   `["redis"]`,
		NodeNames:  `["node-02"]`,
		Metadata:   `{}`,
	}
	require.NoError(t, database.Create(&inc).Error)

	r := chi.NewRouter()
	r.Get("/api/incidents/{id}/report.pdf", handlers.DownloadIncidentReport(database))

	req := httptest.NewRequest(http.MethodGet, "/api/incidents/"+incID+"/report.pdf", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/pdf", rr.Header().Get("Content-Type"))
	assert.True(t, strings.HasPrefix(rr.Body.String(), "%PDF-"), "body must start with PDF magic bytes")
}

func TestDownloadIncidentReport_WithAIAnalysis(t *testing.T) {
	database := newTestDB(t)

	opened := time.Now().UTC().Add(-time.Hour)
	resolved := time.Now().UTC()
	incNoAI := models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   opened,
		ResolvedAt: &resolved,
		Status:     "resolved",
		Confidence: "confirmed",
		Title:      "Test incident",
		Services:   `["svc"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{}`,
	}
	longAnalysis := strings.Repeat("A", 500)
	incWithAI := models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   opened,
		ResolvedAt: &resolved,
		Status:     "resolved",
		Confidence: "confirmed",
		Title:      "Test incident",
		Services:   `["svc"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{"ai_analysis":"` + longAnalysis + `","ai_model":"llama3"}`,
	}
	require.NoError(t, database.Create(&incNoAI).Error)
	require.NoError(t, database.Create(&incWithAI).Error)

	r := chi.NewRouter()
	r.Get("/api/incidents/{id}/report.pdf", handlers.DownloadIncidentReport(database))

	reqNoAI := httptest.NewRequest(http.MethodGet, "/api/incidents/"+incNoAI.ID+"/report.pdf", nil)
	rrNoAI := httptest.NewRecorder()
	r.ServeHTTP(rrNoAI, reqNoAI)
	require.Equal(t, http.StatusOK, rrNoAI.Code)

	reqWithAI := httptest.NewRequest(http.MethodGet, "/api/incidents/"+incWithAI.ID+"/report.pdf", nil)
	rrWithAI := httptest.NewRecorder()
	r.ServeHTTP(rrWithAI, reqWithAI)
	require.Equal(t, http.StatusOK, rrWithAI.Code)

	assert.Greater(t, rrWithAI.Body.Len(), rrNoAI.Body.Len())
}
