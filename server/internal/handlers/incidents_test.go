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
