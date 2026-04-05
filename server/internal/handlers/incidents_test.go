package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListIncidents(t *testing.T) {
	database := newTestDB(t)

	now := time.Now().UTC()
	inc := models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   now,
		Status:     "open",
		Confidence: "confirmed",
		Title:      "nginx down",
		Services:   `["nginx"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{}`,
	}
	require.NoError(t, database.Create(&inc).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/incidents", nil)
	rr := httptest.NewRecorder()
	handlers.ListIncidents(database)(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp struct {
		Incidents []models.Incident `json:"incidents"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Len(t, resp.Incidents, 1)
	assert.Equal(t, inc.ID, resp.Incidents[0].ID)
}

func TestGetIncident_NotFound(t *testing.T) {
	database := newTestDB(t)

	r := chi.NewRouter()
	r.Get("/api/incidents/{id}", handlers.GetIncident(database))

	req := httptest.NewRequest(http.MethodGet, "/api/incidents/nonexistent", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetIncident_Found(t *testing.T) {
	database := newTestDB(t)

	incident := models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   time.Now().UTC(),
		Status:     "open",
		Confidence: "confirmed",
		Title:      "traefik down",
		Services:   `["traefik"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{}`,
	}
	require.NoError(t, database.Create(&incident).Error)

	r := chi.NewRouter()
	r.Get("/api/incidents/{id}", handlers.GetIncident(database))

	req := httptest.NewRequest(http.MethodGet, "/api/incidents/"+incident.ID, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp struct {
		Incident models.Incident `json:"incident"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, incident.ID, resp.Incident.ID)
	assert.Equal(t, incident.Title, resp.Incident.Title)
}

func TestListIncidents_EscapesServiceWildcards(t *testing.T) {
	database := newTestDB(t)

	require.NoError(t, database.Create(&models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   time.Now().UTC(),
		Status:     "open",
		Confidence: "confirmed",
		Title:      "svc_1 down",
		Services:   `["svc_1"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{}`,
	}).Error)
	require.NoError(t, database.Create(&models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   time.Now().UTC(),
		Status:     "open",
		Confidence: "confirmed",
		Title:      "svcA1 down",
		Services:   `["svcA1"]`,
		NodeNames:  `["node-02"]`,
		Metadata:   `{}`,
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/incidents?service=svc_1", nil)
	rr := httptest.NewRecorder()
	handlers.ListIncidents(database)(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp struct {
		Incidents []models.Incident `json:"incidents"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Incidents, 1)
	assert.Equal(t, `["svc_1"]`, resp.Incidents[0].Services)
}
