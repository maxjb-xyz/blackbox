package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestListIncidents_PaginatesWithHasMore(t *testing.T) {
	database := newTestDB(t)

	now := time.Now().UTC()
	older := models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   now,
		Status:     "open",
		Confidence: "confirmed",
		Title:      "older incident",
		Services:   `["nginx"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{}`,
	}
	newer := models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   now.Add(time.Second),
		Status:     "open",
		Confidence: "confirmed",
		Title:      "newer incident",
		Services:   `["traefik"]`,
		NodeNames:  `["node-02"]`,
		Metadata:   `{}`,
	}
	require.NoError(t, database.Create(&older).Error)
	require.NoError(t, database.Create(&newer).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/incidents?limit=1", nil)
	rr := httptest.NewRecorder()
	handlers.ListIncidents(database)(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp struct {
		Incidents []models.Incident `json:"incidents"`
		HasMore   bool              `json:"has_more"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Len(t, resp.Incidents, 1)
	assert.Equal(t, newer.ID, resp.Incidents[0].ID)
	assert.True(t, resp.HasMore)
}

func TestGetIncidentSummary(t *testing.T) {
	database := newTestDB(t)

	require.NoError(t, database.Create(&models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   time.Now().UTC().Add(time.Minute),
		Status:     "open",
		Confidence: "confirmed",
		Title:      "confirmed open incident",
		Services:   `["nginx"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{}`,
	}).Error)
	require.NoError(t, database.Create(&models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   time.Now().UTC(),
		Status:     "open",
		Confidence: "suspected",
		Title:      "suspected open incident",
		Services:   `["traefik"]`,
		NodeNames:  `["node-02"]`,
		Metadata:   `{}`,
	}).Error)
	require.NoError(t, database.Create(&models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   time.Now().UTC().Add(-time.Minute),
		Status:     "resolved",
		Confidence: "confirmed",
		Title:      "resolved incident",
		Services:   `["redis"]`,
		NodeNames:  `["node-03"]`,
		Metadata:   `{}`,
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/incidents/summary", nil)
	rr := httptest.NewRecorder()
	handlers.GetIncidentSummary(database)(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp struct {
		OpenCount        int64 `json:"open_count"`
		HasConfirmedOpen bool  `json:"has_confirmed_open"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.EqualValues(t, 2, resp.OpenCount)
	assert.True(t, resp.HasConfirmedOpen)
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

func TestListIncidentMembership(t *testing.T) {
	database := newTestDB(t)

	entryA := types.Entry{ID: ulid.Make().String(), Timestamp: time.Now().UTC()}
	entryB := types.Entry{ID: ulid.Make().String(), Timestamp: time.Now().UTC()}
	require.NoError(t, database.Create(&entryA).Error)
	require.NoError(t, database.Create(&entryB).Error)

	openIncident := models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   time.Now().UTC().Add(time.Minute),
		Status:     "open",
		Confidence: "confirmed",
		Title:      "open incident",
		Services:   `["svc"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{}`,
	}
	resolvedIncident := models.Incident{
		ID:         ulid.Make().String(),
		OpenedAt:   time.Now().UTC(),
		Status:     "resolved",
		Confidence: "suspected",
		Title:      "resolved incident",
		Services:   `["svc"]`,
		NodeNames:  `["node-01"]`,
		Metadata:   `{}`,
	}
	require.NoError(t, database.Create(&openIncident).Error)
	require.NoError(t, database.Create(&resolvedIncident).Error)
	require.NoError(t, database.Create(&models.IncidentEntry{
		IncidentID: openIncident.ID,
		EntryID:    entryA.ID,
		Role:       "cause",
		Score:      90,
	}).Error)
	require.NoError(t, database.Create(&models.IncidentEntry{
		IncidentID: resolvedIncident.ID,
		EntryID:    entryB.ID,
		Role:       "cause",
		Score:      80,
	}).Error)

	body := bytes.NewBufferString(`{"entry_ids":["` + entryA.ID + `","` + entryB.ID + `","` + entryA.ID + `"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/incidents/membership", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handlers.ListIncidentMembership(database)(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp struct {
		Memberships map[string]struct {
			ID         string `json:"id"`
			Confidence string `json:"confidence"`
		} `json:"memberships"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Memberships, 1)
	assert.Equal(t, openIncident.ID, resp.Memberships[entryA.ID].ID)
	assert.Equal(t, openIncident.Confidence, resp.Memberships[entryA.ID].Confidence)
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
