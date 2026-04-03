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

	for i := 0; i < 5; i++ {
		entry := types.Entry{
			ID:        ulid.Make().String(),
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Second),
			NodeName:  "homelab-01",
			Source:    "docker",
			Event:     "start",
			Content:   fmt.Sprintf("entry %d", i),
		}
		require.NoError(t, database.Create(&entry).Error)
		time.Sleep(time.Millisecond)
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
