package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/shared/types"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListEntries_Empty(t *testing.T) {
	database := newTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/entries", nil)
	rr := httptest.NewRecorder()

	handlers.ListEntries(database)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var resp struct {
		Entries    []types.Entry `json:"entries"`
		NextCursor string        `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Len(t, resp.Entries, 0)
}

func TestListEntries_Pagination(t *testing.T) {
	database := newTestDB(t)

	base := time.Date(2026, 4, 4, 20, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		entry := types.Entry{
			ID:        fmt.Sprintf("01TESTULIDENTRY%d", i),
			Timestamp: base.Add(time.Duration(i) * time.Second),
			NodeName:  "homelab-01",
			Source:    "docker",
			Event:     "start",
			Content:   fmt.Sprintf("entry %d", i),
		}
		require.NoError(t, database.Create(&entry).Error)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/entries?limit=3", nil)
	rr := httptest.NewRecorder()

	handlers.ListEntries(database)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var resp struct {
		Entries    []types.Entry `json:"entries"`
		NextCursor string        `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Entries, 3)
	assert.NotEmpty(t, resp.NextCursor)
	assert.Equal(t, "entry 4", resp.Entries[0].Content)
	assert.Equal(t, "entry 3", resp.Entries[1].Content)
	assert.Equal(t, "entry 2", resp.Entries[2].Content)

	req = httptest.NewRequest(http.MethodGet, "/api/entries?limit=3&cursor="+resp.NextCursor, nil)
	rr = httptest.NewRecorder()

	handlers.ListEntries(database)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp2 struct {
		Entries    []types.Entry `json:"entries"`
		NextCursor string        `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp2))
	require.Len(t, resp2.Entries, 2)
	assert.Equal(t, "entry 1", resp2.Entries[0].Content)
	assert.Equal(t, "entry 0", resp2.Entries[1].Content)
	assert.Empty(t, resp2.NextCursor)
}

func TestListEntries_FilterByNode(t *testing.T) {
	database := newTestDB(t)

	require.NoError(t, database.Create(&types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "node-a",
		Source:    "docker",
		Event:     "start",
		Content:   "a",
	}).Error)
	time.Sleep(time.Millisecond)
	require.NoError(t, database.Create(&types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "node-b",
		Source:    "docker",
		Event:     "start",
		Content:   "b",
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/entries?node=node-a", nil)
	rr := httptest.NewRecorder()

	handlers.ListEntries(database)(rr, req)

	var resp struct {
		Entries []types.Entry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Entries, 1)
	assert.Equal(t, "node-a", resp.Entries[0].NodeName)
}

func TestListEntries_FilterByTimeRange(t *testing.T) {
	database := newTestDB(t)

	base := time.Date(2026, 4, 4, 20, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		entry := types.Entry{
			ID:        fmt.Sprintf("01TIMERANGEENTRY%d", i),
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			NodeName:  "homelab-01",
			Source:    "docker",
			Event:     "start",
			Content:   fmt.Sprintf("entry %d", i),
		}
		require.NoError(t, database.Create(&entry).Error)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/entries?time_start="+base.Add(time.Minute).Format(time.RFC3339Nano)+"&time_end="+base.Add(2*time.Minute).Format(time.RFC3339Nano),
		nil,
	)
	rr := httptest.NewRecorder()

	handlers.ListEntries(database)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp struct {
		Entries []types.Entry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Entries, 2)
	assert.Equal(t, "entry 2", resp.Entries[0].Content)
	assert.Equal(t, "entry 1", resp.Entries[1].Content)
}

func TestListEntries_HidesHeartbeatsWithHideHeartbeatTrue(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&types.Entry{ID: "01", Source: "agent", Event: "heartbeat", NodeName: "n1", Content: "hb"}).Error)
	require.NoError(t, database.Create(&types.Entry{ID: "02", Source: "docker", Event: "start", NodeName: "n1", Content: "svc start"}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/entries?hide_heartbeat=true", nil)
	w := httptest.NewRecorder()

	handlers.ListEntries(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Entries    []types.Entry `json:"entries"`
		NextCursor string        `json:"next_cursor"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp.Entries, 1)
	assert.Equal(t, "02", resp.Entries[0].ID)
}

func TestListEntries_ShowsHeartbeatsWhenNotFiltered(t *testing.T) {
	database := newTestDB(t)
	require.NoError(t, database.Create(&types.Entry{ID: "01", Source: "agent", Event: "heartbeat", NodeName: "n1", Content: "hb"}).Error)
	require.NoError(t, database.Create(&types.Entry{ID: "02", Source: "docker", Event: "start", NodeName: "n1", Content: "svc start"}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/entries", nil)
	w := httptest.NewRecorder()

	handlers.ListEntries(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Entries    []types.Entry `json:"entries"`
		NextCursor string        `json:"next_cursor"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp.Entries, 2)
}

func TestListEntries_OrdersByTimestampThenID(t *testing.T) {
	database := newTestDB(t)
	base := time.Date(2026, 4, 4, 19, 0, 0, 0, time.UTC)

	require.NoError(t, database.Create(&types.Entry{
		ID:        "04",
		Timestamp: base.Add(time.Second),
		NodeName:  "n1",
		Source:    "docker",
		Event:     "restart",
		Content:   "newer timestamp",
	}).Error)
	require.NoError(t, database.Create(&types.Entry{
		ID:        "03",
		Timestamp: base,
		NodeName:  "n1",
		Source:    "docker",
		Event:     "stop",
		Content:   "same timestamp highest id",
	}).Error)
	require.NoError(t, database.Create(&types.Entry{
		ID:        "02",
		Timestamp: base,
		NodeName:  "n1",
		Source:    "docker",
		Event:     "start",
		Content:   "same timestamp middle id",
	}).Error)
	require.NoError(t, database.Create(&types.Entry{
		ID:        "01",
		Timestamp: base,
		NodeName:  "n1",
		Source:    "docker",
		Event:     "create",
		Content:   "same timestamp lowest id",
	}).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/entries?limit=2", nil)
	w := httptest.NewRecorder()

	handlers.ListEntries(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Entries    []types.Entry `json:"entries"`
		NextCursor string        `json:"next_cursor"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp.Entries, 2)
	assert.Equal(t, "04", resp.Entries[0].ID)
	assert.Equal(t, "03", resp.Entries[1].ID)
	require.NotEmpty(t, resp.NextCursor)

	req = httptest.NewRequest(http.MethodGet, "/api/entries?limit=2&cursor="+resp.NextCursor, nil)
	w = httptest.NewRecorder()

	handlers.ListEntries(database)(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp2 struct {
		Entries    []types.Entry `json:"entries"`
		NextCursor string        `json:"next_cursor"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp2))
	require.Len(t, resp2.Entries, 2)
	assert.Equal(t, "02", resp2.Entries[0].ID)
	assert.Equal(t, "01", resp2.Entries[1].ID)
	assert.Empty(t, resp2.NextCursor)
}

func TestListEntries_InvalidCursor(t *testing.T) {
	database := newTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/entries?cursor=not-a-cursor", nil)
	w := httptest.NewRecorder()

	handlers.ListEntries(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetEntry_NotFound(t *testing.T) {
	database := newTestDB(t)

	router := chi.NewRouter()
	router.Get("/api/entries/{id}", handlers.GetEntry(database))

	req := httptest.NewRequest(http.MethodGet, "/api/entries/nonexistent", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetEntry_Found(t *testing.T) {
	database := newTestDB(t)

	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Event:     "die",
		Content:   "container nginx died",
	}
	require.NoError(t, database.Create(&entry).Error)

	router := chi.NewRouter()
	router.Get("/api/entries/{id}", handlers.GetEntry(database))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/entries/%s", entry.ID), nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var got types.Entry
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, entry.ID, got.ID)
}

func TestListEntries_FTSSearch(t *testing.T) {
	// newTestDB calls db.Init which runs EnsureEntriesFTS, so FTS is ready.
	database := newTestDB(t)

	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Event:     "start",
		Service:   "webapp",
		Content:   "uniquecontentword started successfully",
	}
	require.NoError(t, database.Create(&entry).Error)

	// Matching term — entry should be returned.
	req := httptest.NewRequest(http.MethodGet, "/api/entries?q=uniquecontentword", nil)
	rr := httptest.NewRecorder()
	handlers.ListEntries(database)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp struct {
		Entries []types.Entry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Entries, 1)
	assert.Equal(t, entry.ID, resp.Entries[0].ID)

	// Non-matching term — no results.
	req = httptest.NewRequest(http.MethodGet, "/api/entries?q=termthatdoesnotexist", nil)
	rr = httptest.NewRecorder()
	handlers.ListEntries(database)(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp2 struct {
		Entries []types.Entry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp2))
	assert.Len(t, resp2.Entries, 0)
}
