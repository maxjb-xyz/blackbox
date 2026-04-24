package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"blackbox/server/internal/auth"
	dbpkg "blackbox/server/internal/db"
	"blackbox/server/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newPlaybookTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := dbpkg.Init(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, err := database.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	return database
}

func adminPlaybookRequest(method, target string, body *bytes.Reader) *http.Request {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, body)
	}
	return req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, &auth.Claims{
		UserID:  "admin1",
		IsAdmin: true,
	}))
}

func playbookBody(t *testing.T, overrides map[string]any) *bytes.Reader {
	t.Helper()
	payload := map[string]any{
		"name":            "Nextcloud restart",
		"service_pattern": "docker:nextcloud",
		"priority":        0,
		"content_md":      "# Restart\n\n1. `docker restart nextcloud`\n",
		"enabled":         true,
	}
	for k, v := range overrides {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return bytes.NewReader(raw)
}

func withPlaybookIDParam(req *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestListPlaybooks_EmptyReturnsJSONArray(t *testing.T) {
	database := newPlaybookTestDB(t)
	w := httptest.NewRecorder()
	ListPlaybooks(database)(w, adminPlaybookRequest(http.MethodGet, "/api/admin/playbooks", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	var got []models.Playbook
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Empty(t, got)
}

func TestCreatePlaybook_Valid(t *testing.T) {
	database := newPlaybookTestDB(t)
	req := adminPlaybookRequest(http.MethodPost, "/api/admin/playbooks", playbookBody(t, nil))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreatePlaybook(database)(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var got models.Playbook
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.NotEmpty(t, got.ID)
	assert.Equal(t, "Nextcloud restart", got.Name)
	assert.Equal(t, "docker:nextcloud", got.ServicePattern)
	assert.True(t, got.Enabled)
}

func TestCreatePlaybook_RejectsInvalidGlob(t *testing.T) {
	database := newPlaybookTestDB(t)
	req := adminPlaybookRequest(http.MethodPost, "/api/admin/playbooks", playbookBody(t, map[string]any{
		"service_pattern": "[unclosed",
	}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreatePlaybook(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "service_pattern")
}

func TestCreatePlaybook_RejectsMissingFields(t *testing.T) {
	database := newPlaybookTestDB(t)

	cases := map[string]map[string]any{
		"missing name":    {"name": ""},
		"missing pattern": {"service_pattern": ""},
		"missing content": {"content_md": ""},
	}
	for label, overrides := range cases {
		t.Run(label, func(t *testing.T) {
			req := adminPlaybookRequest(http.MethodPost, "/api/admin/playbooks", playbookBody(t, overrides))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			CreatePlaybook(database)(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code, label)
		})
	}
}

func TestCreatePlaybook_RejectsOversizedContent(t *testing.T) {
	database := newPlaybookTestDB(t)
	big := strings.Repeat("x", playbookMaxContentBytes+1)
	req := adminPlaybookRequest(http.MethodPost, "/api/admin/playbooks", playbookBody(t, map[string]any{
		"content_md": big,
	}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreatePlaybook(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdatePlaybook_Valid(t *testing.T) {
	database := newPlaybookTestDB(t)

	// seed
	createReq := adminPlaybookRequest(http.MethodPost, "/api/admin/playbooks", playbookBody(t, nil))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	CreatePlaybook(database)(createW, createReq)
	require.Equal(t, http.StatusCreated, createW.Code)
	var created models.Playbook
	require.NoError(t, json.NewDecoder(createW.Body).Decode(&created))

	// update
	updReq := adminPlaybookRequest(http.MethodPut, "/api/admin/playbooks/"+created.ID, playbookBody(t, map[string]any{
		"name":     "Renamed runbook",
		"priority": 42,
	}))
	updReq.Header.Set("Content-Type", "application/json")
	updReq = withPlaybookIDParam(updReq, created.ID)
	updW := httptest.NewRecorder()
	UpdatePlaybook(database)(updW, updReq)

	assert.Equal(t, http.StatusOK, updW.Code)
	var updated models.Playbook
	require.NoError(t, json.NewDecoder(updW.Body).Decode(&updated))
	assert.Equal(t, "Renamed runbook", updated.Name)
	assert.Equal(t, 42, updated.Priority)
	assert.Equal(t, created.ID, updated.ID)
}

func TestUpdatePlaybook_NotFound(t *testing.T) {
	database := newPlaybookTestDB(t)
	req := adminPlaybookRequest(http.MethodPut, "/api/admin/playbooks/missing", playbookBody(t, nil))
	req.Header.Set("Content-Type", "application/json")
	req = withPlaybookIDParam(req, "missing")
	w := httptest.NewRecorder()
	UpdatePlaybook(database)(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeletePlaybook(t *testing.T) {
	database := newPlaybookTestDB(t)

	createReq := adminPlaybookRequest(http.MethodPost, "/api/admin/playbooks", playbookBody(t, nil))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	CreatePlaybook(database)(createW, createReq)
	var created models.Playbook
	require.NoError(t, json.NewDecoder(createW.Body).Decode(&created))

	delReq := adminPlaybookRequest(http.MethodDelete, "/api/admin/playbooks/"+created.ID, nil)
	delReq = withPlaybookIDParam(delReq, created.ID)
	delW := httptest.NewRecorder()
	DeletePlaybook(database)(delW, delReq)
	assert.Equal(t, http.StatusNoContent, delW.Code)

	// second delete → 404
	delReq2 := adminPlaybookRequest(http.MethodDelete, "/api/admin/playbooks/"+created.ID, nil)
	delReq2 = withPlaybookIDParam(delReq2, created.ID)
	delW2 := httptest.NewRecorder()
	DeletePlaybook(database)(delW2, delReq2)
	assert.Equal(t, http.StatusNotFound, delW2.Code)
}

func TestPlaybook_RequiresAdmin(t *testing.T) {
	database := newPlaybookTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/playbooks", nil)
	// No admin claims on context.
	w := httptest.NewRecorder()
	ListPlaybooks(database)(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
