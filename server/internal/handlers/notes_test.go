package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ctxWithClaims(userID, username string) context.Context {
	claims := &auth.Claims{UserID: userID, Username: username}
	return context.WithValue(context.Background(), auth.ClaimsKey, claims)
}

func TestCreateNote_Success(t *testing.T) {
	database := newTestDB(t)

	userID := ulid.Make().String()

	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Event:     "die",
		Content:   "container nginx died",
	}
	require.NoError(t, database.Create(&entry).Error)

	body, err := json.Marshal(map[string]string{"content": "OOM killer, memcache misconfigured."})
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Post("/api/entries/{id}/notes", handlers.CreateNote(database))

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/entries/%s/notes", entry.ID), bytes.NewReader(body))
	req = req.WithContext(ctxWithClaims(userID, "alice"))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var note models.EntryNote
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &note))
	assert.Equal(t, "OOM killer, memcache misconfigured.", note.Content)
	assert.Equal(t, "alice", note.Username)
	assert.Equal(t, entry.ID, note.EntryID)
}

func TestListNotes_Success(t *testing.T) {
	database := newTestDB(t)

	entryID := ulid.Make().String()
	baseTime := time.Now().UTC().Round(0)
	require.NoError(t, database.Create(&types.Entry{
		ID:        entryID,
		Timestamp: baseTime,
		NodeName:  "h",
		Source:    "docker",
		Event:     "die",
		Content:   "x",
	}).Error)
	require.NoError(t, database.Create(&models.EntryNote{
		ID:        ulid.Make().String(),
		EntryID:   entryID,
		UserID:    "u1",
		Username:  "alice",
		Content:   "note 1",
		CreatedAt: baseTime,
	}).Error)
	require.NoError(t, database.Create(&models.EntryNote{
		ID:        ulid.Make().String(),
		EntryID:   entryID,
		UserID:    "u2",
		Username:  "bob",
		Content:   "note 2",
		CreatedAt: baseTime.Add(time.Millisecond),
	}).Error)

	router := chi.NewRouter()
	router.Get("/api/entries/{id}/notes", handlers.ListNotes(database))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/entries/%s/notes", entryID), nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var notes []models.EntryNote
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &notes))
	require.Len(t, notes, 2)
	assert.Equal(t, "note 1", notes[0].Content)
}

func TestCreateNote_WhitespaceContentRejected(t *testing.T) {
	database := newTestDB(t)

	userID := ulid.Make().String()
	entryID := ulid.Make().String()
	require.NoError(t, database.Create(&types.Entry{
		ID:        entryID,
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Event:     "die",
		Content:   "container nginx died",
	}).Error)

	router := chi.NewRouter()
	router.Post("/api/entries/{id}/notes", handlers.CreateNote(database))

	req := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/api/entries/%s/notes", entryID),
		bytes.NewBufferString(`{"content":"   "}`),
	)
	req = req.WithContext(ctxWithClaims(userID, "alice"))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestCreateNote_EntryNotFound(t *testing.T) {
	database := newTestDB(t)

	userID := ulid.Make().String()

	router := chi.NewRouter()
	router.Post("/api/entries/{id}/notes", handlers.CreateNote(database))

	req := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/api/entries/%s/notes", ulid.Make().String()),
		bytes.NewBufferString(`{"content":"hello"}`),
	)
	req = req.WithContext(ctxWithClaims(userID, "alice"))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestDeleteNote_OwnNote(t *testing.T) {
	database := newTestDB(t)

	userID := ulid.Make().String()
	noteID := ulid.Make().String()
	require.NoError(t, database.Create(&types.Entry{
		ID:        "e1",
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Event:     "die",
		Content:   "container nginx died",
	}).Error)
	require.NoError(t, database.Create(&models.EntryNote{
		ID:        noteID,
		EntryID:   "e1",
		UserID:    userID,
		Username:  "alice",
		Content:   "x",
		CreatedAt: time.Now().UTC(),
	}).Error)

	router := chi.NewRouter()
	router.Delete("/api/notes/{id}", handlers.DeleteNote(database))

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/notes/%s", noteID), nil)
	req = req.WithContext(ctxWithClaims(userID, ""))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
}

func TestDeleteNote_OtherUserForbidden(t *testing.T) {
	database := newTestDB(t)

	noteID := ulid.Make().String()
	require.NoError(t, database.Create(&types.Entry{
		ID:        "e1",
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Event:     "die",
		Content:   "container nginx died",
	}).Error)
	require.NoError(t, database.Create(&models.EntryNote{
		ID:        noteID,
		EntryID:   "e1",
		UserID:    "other-user",
		Username:  "bob",
		Content:   "x",
		CreatedAt: time.Now().UTC(),
	}).Error)

	router := chi.NewRouter()
	router.Delete("/api/notes/{id}", handlers.DeleteNote(database))

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/notes/%s", noteID), nil)
	req = req.WithContext(ctxWithClaims("different-user", ""))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}
