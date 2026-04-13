package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/auth"
	dbpkg "blackbox/server/internal/db"
	"blackbox/server/internal/models"
	"blackbox/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestListNotificationDests_Empty(t *testing.T) {
	database := newNotificationTestDB(t)
	req := adminNotificationRequest(http.MethodGet, "/api/admin/notifications", nil)
	w := httptest.NewRecorder()

	ListNotificationDests(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result []models.NotificationDest
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Empty(t, result)
}

func TestCreateNotificationDest_Valid(t *testing.T) {
	database := newNotificationTestDB(t)
	body, err := json.Marshal(map[string]interface{}{
		"name":    "Discord #alerts",
		"type":    "discord",
		"url":     "https://discord.com/api/webhooks/1234/abcd",
		"events":  []string{notify.EventIncidentOpenedConfirmed, notify.EventIncidentResolved},
		"enabled": true,
	})
	require.NoError(t, err)

	req := adminNotificationRequest(http.MethodPost, "/api/admin/notifications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreateNotificationDest(database)(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var dest models.NotificationDest
	require.NoError(t, json.NewDecoder(w.Body).Decode(&dest))
	assert.Equal(t, "Discord #alerts", dest.Name)
	assert.Equal(t, "discord", dest.Type)
	assert.NotEmpty(t, dest.ID)
}

func TestCreateNotificationDest_InvalidType(t *testing.T) {
	database := newNotificationTestDB(t)
	body, err := json.Marshal(map[string]interface{}{
		"name":   "Bad",
		"type":   "telegram",
		"url":    "https://example.com",
		"events": []string{notify.EventIncidentResolved},
	})
	require.NoError(t, err)

	req := adminNotificationRequest(http.MethodPost, "/api/admin/notifications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreateNotificationDest(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateNotificationDest_InvalidURL(t *testing.T) {
	database := newNotificationTestDB(t)
	body, err := json.Marshal(map[string]interface{}{
		"name":   "Bad",
		"type":   "discord",
		"url":    "not-a-url",
		"events": []string{notify.EventIncidentResolved},
	})
	require.NoError(t, err)

	req := adminNotificationRequest(http.MethodPost, "/api/admin/notifications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreateNotificationDest(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateNotificationDest_NoEvents(t *testing.T) {
	database := newNotificationTestDB(t)
	body, err := json.Marshal(map[string]interface{}{
		"name":   "Bad",
		"type":   "discord",
		"url":    "https://example.com/webhook",
		"events": []string{},
	})
	require.NoError(t, err)

	req := adminNotificationRequest(http.MethodPost, "/api/admin/notifications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreateNotificationDest(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateNotificationDest_Valid(t *testing.T) {
	database := newNotificationTestDB(t)
	require.NoError(t, database.Create(&models.NotificationDest{
		ID:      "dest-upd",
		Name:    "Old",
		Type:    "discord",
		URL:     "https://discord.com/api/webhooks/1/a",
		Events:  `["incident_resolved"]`,
		Enabled: false,
	}).Error)

	body, err := json.Marshal(map[string]interface{}{
		"name":    "New Name",
		"type":    "slack",
		"url":     "https://hooks.slack.com/services/T/B/X",
		"events":  []string{notify.EventIncidentResolved},
		"enabled": true,
	})
	require.NoError(t, err)

	req := adminNotificationRequest(http.MethodPut, "/api/admin/notifications/dest-upd", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withNotificationRouteParam(req, "id", "dest-upd")
	w := httptest.NewRecorder()

	UpdateNotificationDest(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var dest models.NotificationDest
	require.NoError(t, json.NewDecoder(w.Body).Decode(&dest))
	assert.Equal(t, "New Name", dest.Name)
	assert.Equal(t, "slack", dest.Type)
	assert.True(t, dest.Enabled)
}

func TestUpdateNotificationDest_NotFound(t *testing.T) {
	database := newNotificationTestDB(t)
	body, err := json.Marshal(map[string]interface{}{
		"name":   "X",
		"type":   "discord",
		"url":    "https://discord.com/api/webhooks/1/a",
		"events": []string{notify.EventIncidentResolved},
	})
	require.NoError(t, err)

	req := adminNotificationRequest(http.MethodPut, "/api/admin/notifications/nonexistent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withNotificationRouteParam(req, "id", "nonexistent")
	w := httptest.NewRecorder()

	UpdateNotificationDest(database)(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteNotificationDest_Valid(t *testing.T) {
	database := newNotificationTestDB(t)
	require.NoError(t, database.Create(&models.NotificationDest{
		ID:      "dest-del",
		Name:    "To Delete",
		Type:    "ntfy",
		URL:     "https://ntfy.sh/mytopic",
		Events:  `["incident_resolved"]`,
		Enabled: true,
	}).Error)

	req := adminNotificationRequest(http.MethodDelete, "/api/admin/notifications/dest-del", nil)
	req = withNotificationRouteParam(req, "id", "dest-del")
	w := httptest.NewRecorder()

	DeleteNotificationDest(database)(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteNotificationDest_NotFound(t *testing.T) {
	database := newNotificationTestDB(t)
	req := adminNotificationRequest(http.MethodDelete, "/api/admin/notifications/nope", nil)
	req = withNotificationRouteParam(req, "id", "nope")
	w := httptest.NewRecorder()

	DeleteNotificationDest(database)(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTestNotificationDest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	database := newNotificationTestDB(t)
	require.NoError(t, database.Create(&models.NotificationDest{
		ID:      "dest-tst",
		Name:    "Test",
		Type:    "discord",
		URL:     srv.URL,
		Events:  `["incident_resolved"]`,
		Enabled: true,
	}).Error)

	req := adminNotificationRequest(http.MethodPost, "/api/admin/notifications/dest-tst/test", nil)
	req = withNotificationRouteParam(req, "id", "dest-tst")
	w := httptest.NewRecorder()

	TestNotificationDest(database, notify.NewDispatcher(database))(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, true, resp["ok"])
}

func TestTestNotificationDest_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	database := newNotificationTestDB(t)
	require.NoError(t, database.Create(&models.NotificationDest{
		ID:      "dest-tst2",
		Name:    "Failing",
		Type:    "discord",
		URL:     srv.URL,
		Events:  `["incident_resolved"]`,
		Enabled: true,
	}).Error)

	req := adminNotificationRequest(http.MethodPost, "/api/admin/notifications/dest-tst2/test", nil)
	req = withNotificationRouteParam(req, "id", "dest-tst2")
	w := httptest.NewRecorder()

	TestNotificationDest(database, notify.NewDispatcher(database))(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, false, resp["ok"])
	assert.NotEmpty(t, resp["error"])
}

func newNotificationTestDB(t *testing.T) *gorm.DB {
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

func adminNotificationRequest(method, target string, body *bytes.Reader) *http.Request {
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

func withNotificationRouteParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
