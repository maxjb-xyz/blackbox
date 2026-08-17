package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestListNotificationHistory_BoundedFilteredAndSecretFree(t *testing.T) {
	database := newNotificationTestDB(t)
	require.NoError(t, database.Create(&models.NotificationDest{
		ID: "dest-history", Name: "Ops Slack", Type: "slack", URL: "https://hooks.example/secret-token", Enabled: true,
	}).Error)
	now := time.Now().UTC()
	require.NoError(t, database.Create(&models.NotificationLog{
		ID: "log-sent", DestID: "dest-history", IncidentID: "incident-1", Event: "incident_resolved", Decision: "sent", Note: "delivered", CreatedAt: now,
	}).Error)
	require.NoError(t, database.Create(&models.NotificationLog{
		ID: "log-dropped", DestID: "dest-history", IncidentID: "incident-2", Event: "incident_resolved", Decision: "dropped_quiet", Note: "quiet hours", CreatedAt: now.Add(-time.Minute),
	}).Error)

	req := adminNotificationRequest(http.MethodGet, "/api/admin/notifications/history?decision=dropped&limit=1", nil)
	req.URL.RawQuery = "decision=dropped&limit=1"
	w := httptest.NewRecorder()
	ListNotificationHistory(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.NotContains(t, body, "secret-token")
	var response struct {
		Items []struct {
			DestinationID   string `json:"destination_id"`
			DestinationName string `json:"destination_name"`
			Decision        string `json:"decision"`
			Note            string `json:"note"`
		} `json:"items"`
		HasMore bool `json:"has_more"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &response))
	require.Len(t, response.Items, 1)
	assert.Equal(t, "dest-history", response.Items[0].DestinationID)
	assert.Equal(t, "Ops Slack", response.Items[0].DestinationName)
	assert.Equal(t, "dropped_quiet", response.Items[0].Decision)
	assert.Equal(t, "quiet hours", response.Items[0].Note)
	assert.False(t, response.HasMore)
}

func TestListNotificationHistory_HidesFailedNotes(t *testing.T) {
	database := newNotificationTestDB(t)
	require.NoError(t, database.Create(&models.NotificationDest{
		ID: "dest-failed", Name: "Ops Slack", Type: "slack", URL: "https://hooks.example/secret-token", Enabled: true,
	}).Error)
	require.NoError(t, database.Create(&models.NotificationLog{
		ID: "log-failed", DestID: "dest-failed", IncidentID: "incident-1", Event: "incident_opened", Decision: "failed",
		Note: "Post \"https://hooks.example/services/T000/B000/super-secret-token\": 404 Not Found", CreatedAt: time.Now().UTC(),
	}).Error)

	req := adminNotificationRequest(http.MethodGet, "/api/admin/notifications/history?decision=failed", nil)
	req.URL.RawQuery = "decision=failed"
	w := httptest.NewRecorder()
	ListNotificationHistory(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.NotContains(t, body, "super-secret-token")
	assert.NotContains(t, body, "hooks.example/services")
	var response struct {
		Items []struct {
			Note string `json:"note"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &response))
	require.Len(t, response.Items, 1)
	assert.Empty(t, response.Items[0].Note)
}

func TestListNotificationHistory_CompositeCursorKeepsSameTimestampRows(t *testing.T) {
	database := newNotificationTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	for _, id := range []string{"log-c", "log-b", "log-a"} {
		require.NoError(t, database.Create(&models.NotificationLog{
			ID: id, DestID: "dest", IncidentID: "incident", Event: "incident_opened", Decision: "sent", CreatedAt: now,
		}).Error)
	}

	req := adminNotificationRequest(http.MethodGet, "/api/admin/notifications/history?limit=1", nil)
	req.URL.RawQuery = "limit=1"
	w := httptest.NewRecorder()
	ListNotificationHistory(database)(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var first notificationHistoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &first))
	require.Len(t, first.Items, 1)
	require.Equal(t, "log-c", first.Items[0].ID)
	require.NotEmpty(t, first.NextBefore)

	req = adminNotificationRequest(http.MethodGet, "/api/admin/notifications/history", nil)
	req.URL.RawQuery = "limit=2&before=" + first.NextBefore
	w = httptest.NewRecorder()
	ListNotificationHistory(database)(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var second notificationHistoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &second))
	require.Len(t, second.Items, 2)
	assert.Equal(t, "log-b", second.Items[0].ID)
	assert.Equal(t, "log-a", second.Items[1].ID)
}

func TestListNotificationHistory_RejectsUnboundedLimit(t *testing.T) {
	database := newNotificationTestDB(t)
	req := adminNotificationRequest(http.MethodGet, "/api/admin/notifications/history?limit=201", nil)
	req.URL.RawQuery = "limit=201"
	w := httptest.NewRecorder()
	ListNotificationHistory(database)(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
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

func TestValidatePolicyFields(t *testing.T) {
	bad := []struct {
		name string
		p    policyInput
	}{
		{"bad start", policyInput{QuietHoursEnabled: true, QuietHoursStart: "9am", QuietHoursEnd: "07:00", QuietHoursMode: "drop"}},
		{"bad mode", policyInput{QuietHoursEnabled: true, QuietHoursStart: "22:00", QuietHoursEnd: "07:00", QuietHoursMode: "mute"}},
		{"rate count zero", policyInput{RateLimitEnabled: true, RateLimitCount: 0, RateLimitUnit: "hour"}},
		{"rate bad unit", policyInput{RateLimitEnabled: true, RateLimitCount: 3, RateLimitUnit: "week"}},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if err := validatePolicy(c.p); err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
		})
	}

	ok := policyInput{
		QuietHoursEnabled: true, QuietHoursStart: "22:00", QuietHoursEnd: "07:00", QuietHoursMode: "defer",
		RateLimitEnabled: true, RateLimitCount: 5, RateLimitUnit: "day",
	}
	if err := validatePolicy(ok); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestCreateNotificationDest_WithPolicy(t *testing.T) {
	database := newNotificationTestDB(t)
	body, err := json.Marshal(map[string]interface{}{
		"name":                "Phone push",
		"type":                "ntfy",
		"url":                 "https://ntfy.sh/mytopic",
		"events":              []string{notify.EventIncidentOpenedConfirmed},
		"enabled":             true,
		"quiet_hours_enabled": true,
		"quiet_hours_start":   "22:00",
		"quiet_hours_end":     "07:00",
		"quiet_hours_mode":    "defer",
		"rate_limit_enabled":  true,
		"rate_limit_count":    5,
		"rate_limit_unit":     "hour",
	})
	require.NoError(t, err)

	req := adminNotificationRequest(http.MethodPost, "/api/admin/notifications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreateNotificationDest(database)(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var dest models.NotificationDest
	require.NoError(t, json.NewDecoder(w.Body).Decode(&dest))
	assert.True(t, dest.QuietHoursEnabled)
	assert.Equal(t, "defer", dest.QuietHoursMode)
	assert.Equal(t, "22:00", dest.QuietHoursStart)
	assert.True(t, dest.RateLimitEnabled)
	assert.Equal(t, 5, dest.RateLimitCount)
	assert.Equal(t, "hour", dest.RateLimitUnit)
}

func TestCreateNotificationDest_InvalidPolicy(t *testing.T) {
	database := newNotificationTestDB(t)
	body, err := json.Marshal(map[string]interface{}{
		"name":                "Bad policy",
		"type":                "ntfy",
		"url":                 "https://ntfy.sh/mytopic",
		"events":              []string{notify.EventIncidentOpenedConfirmed},
		"enabled":             true,
		"quiet_hours_enabled": true,
		"quiet_hours_start":   "9am",
		"quiet_hours_end":     "07:00",
		"quiet_hours_mode":    "drop",
	})
	require.NoError(t, err)

	req := adminNotificationRequest(http.MethodPost, "/api/admin/notifications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreateNotificationDest(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateNotificationDest_WithPolicy(t *testing.T) {
	database := newNotificationTestDB(t)
	require.NoError(t, database.Create(&models.NotificationDest{
		ID:      "dest-pol",
		Name:    "Phone",
		Type:    "ntfy",
		URL:     "https://ntfy.sh/mytopic",
		Events:  `["incident_opened_confirmed"]`,
		Enabled: true,
	}).Error)

	body, err := json.Marshal(map[string]interface{}{
		"name":                "Phone",
		"type":                "ntfy",
		"url":                 "https://ntfy.sh/mytopic",
		"events":              []string{notify.EventIncidentOpenedConfirmed},
		"enabled":             true,
		"quiet_hours_enabled": true,
		"quiet_hours_start":   "23:30",
		"quiet_hours_end":     "06:15",
		"quiet_hours_mode":    "defer",
		"rate_limit_enabled":  true,
		"rate_limit_count":    3,
		"rate_limit_unit":     "day",
	})
	require.NoError(t, err)

	req := adminNotificationRequest(http.MethodPut, "/api/admin/notifications/dest-pol", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withNotificationRouteParam(req, "id", "dest-pol")
	w := httptest.NewRecorder()

	UpdateNotificationDest(database)(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var dest models.NotificationDest
	require.NoError(t, json.NewDecoder(w.Body).Decode(&dest))
	assert.True(t, dest.QuietHoursEnabled)
	assert.Equal(t, "23:30", dest.QuietHoursStart)
	assert.Equal(t, "06:15", dest.QuietHoursEnd)
	assert.Equal(t, "defer", dest.QuietHoursMode)
	assert.True(t, dest.RateLimitEnabled)
	assert.Equal(t, 3, dest.RateLimitCount)
	assert.Equal(t, "day", dest.RateLimitUnit)
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
