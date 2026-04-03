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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCreateEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setup          func(t *testing.T, database *gorm.DB)
		body           string
		wantStatus     int
		checkTimestamp bool
		assertEntry    func(t *testing.T, entry types.Entry)
	}{
		{
			name:       "creates manual entry with explicit timestamp and services",
			body:       `{"title":"Nightly backup completed","note":"manual run","services":["postgres","storage"],"timestamp":"2026-04-02T10:15:30Z"}`,
			wantStatus: http.StatusCreated,
			assertEntry: func(t *testing.T, entry types.Entry) {
				t.Helper()
				assert.Equal(t, "api", entry.NodeName)
				assert.Equal(t, "api", entry.Source)
				assert.Equal(t, "manual", entry.Event)
				assert.Equal(t, "postgres", entry.Service)
				assert.Equal(t, "Nightly backup completed", entry.Content)
				assert.Equal(t, time.Date(2026, 4, 2, 10, 15, 30, 0, time.UTC), entry.Timestamp)

				var meta map[string]interface{}
				require.NoError(t, json.Unmarshal([]byte(entry.Metadata), &meta))
				assert.Equal(t, "manual run", meta["note"])
				assert.Equal(t, []interface{}{"postgres", "storage"}, meta["services"])
			},
		},
		{
			name:       "normalizes aliased services before saving",
			body:       `{"title":"Failover completed","note":"manual run","services":["postgres-primary"],"timestamp":"2026-04-02T10:15:30Z"}`,
			wantStatus: http.StatusCreated,
			setup: func(t *testing.T, database *gorm.DB) {
				t.Helper()
				require.NoError(t, database.Create(&models.ServiceAlias{
					Canonical: "postgres",
					Alias:     "postgres-primary",
				}).Error)
			},
			assertEntry: func(t *testing.T, entry types.Entry) {
				t.Helper()
				assert.Equal(t, "postgres", entry.Service)
				assert.Equal(t, "Failover completed", entry.Content)
				assert.Equal(t, "api", entry.NodeName)

				var meta map[string]interface{}
				require.NoError(t, json.Unmarshal([]byte(entry.Metadata), &meta))
				assert.Equal(t, []interface{}{"postgres"}, meta["services"])
				assert.Equal(t, "manual run", meta["note"])
			},
		},
		{
			name:       "ignores blank normalized services",
			body:       `{"title":"Failover completed","note":"manual run","services":["   ","postgres-primary"],"timestamp":"2026-04-02T10:15:30Z"}`,
			wantStatus: http.StatusCreated,
			setup: func(t *testing.T, database *gorm.DB) {
				t.Helper()
				require.NoError(t, database.Create(&models.ServiceAlias{
					Canonical: "postgres",
					Alias:     "postgres-primary",
				}).Error)
			},
			assertEntry: func(t *testing.T, entry types.Entry) {
				t.Helper()
				assert.Equal(t, "postgres", entry.Service)

				var meta map[string]interface{}
				require.NoError(t, json.Unmarshal([]byte(entry.Metadata), &meta))
				assert.Equal(t, []interface{}{"postgres"}, meta["services"])
			},
		},
		{
			name:           "uses server time when timestamp omitted",
			body:           `{"title":"Ad hoc check","note":""}`,
			wantStatus:     http.StatusCreated,
			checkTimestamp: true,
			assertEntry: func(t *testing.T, entry types.Entry) {
				t.Helper()
				assert.Equal(t, "", entry.Service)

				var meta map[string]interface{}
				require.NoError(t, json.Unmarshal([]byte(entry.Metadata), &meta))
				assert.Equal(t, "", meta["note"])
				assert.Equal(t, []interface{}{}, meta["services"])
			},
		},
		{
			name:       "rejects invalid timestamp",
			body:       `{"title":"Ad hoc check","note":"","timestamp":"not-a-time"}`,
			wantStatus: http.StatusBadRequest,
			assertEntry: func(t *testing.T, entry types.Entry) {
				t.Helper()
			},
		},
		{
			name:       "rejects missing title",
			body:       `{"note":"missing title"}`,
			wantStatus: http.StatusBadRequest,
			assertEntry: func(t *testing.T, entry types.Entry) {
				t.Helper()
			},
		},
		{
			name:       "rejects whitespace-only title",
			body:       `{"title":"   ","note":"missing title"}`,
			wantStatus: http.StatusBadRequest,
			assertEntry: func(t *testing.T, entry types.Entry) {
				t.Helper()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			database := newTestDB(t)
			if tt.setup != nil {
				tt.setup(t, database)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/entries", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			before := time.Now().UTC()
			handlers.CreateEntry(database)(rr, req)
			after := time.Now().UTC()

			require.Equal(t, tt.wantStatus, rr.Code, rr.Body.String())

			var entries []types.Entry
			require.NoError(t, database.Find(&entries).Error)

			if tt.wantStatus != http.StatusCreated {
				assert.Empty(t, entries)
				return
			}

			require.Len(t, entries, 1)

			var responseEntry types.Entry
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &responseEntry))
			assert.Equal(t, entries[0].ID, responseEntry.ID)

			if tt.checkTimestamp {
				assert.True(t, entries[0].Timestamp.After(before) || entries[0].Timestamp.Equal(before))
				assert.True(t, entries[0].Timestamp.Before(after) || entries[0].Timestamp.Equal(after))
			}

			tt.assertEntry(t, entries[0])
		})
	}
}
