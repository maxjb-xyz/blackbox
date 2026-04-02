package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/handlers"
	"blackbox/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIngest_SavesEntry(t *testing.T) {
	database := newTestDB(t)

	entry := types.Entry{
		ID:        "01TESTULIDENTRY1",
		Timestamp: time.Now().UTC(),
		NodeName:  "homelab-01",
		Source:    "docker",
		Service:   "nginx",
		Event:     "start",
		Content:   "Container nginx started",
	}
	body, _ := json.Marshal(entry)
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.Ingest(database)(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var saved types.Entry
	require.NoError(t, database.First(&saved, "id = ?", entry.ID).Error)
	assert.Equal(t, "homelab-01", saved.NodeName)
	assert.Equal(t, "docker", saved.Source)
}

func TestIngest_RejectsMissingID(t *testing.T) {
	database := newTestDB(t)

	entry := types.Entry{NodeName: "node1", Source: "docker"}
	body, _ := json.Marshal(entry)
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.Ingest(database)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
