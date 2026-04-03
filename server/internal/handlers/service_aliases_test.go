package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"blackbox/server/internal/handlers"
	"blackbox/server/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceAliasHandlers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		run    func(t *testing.T)
		verify func(t *testing.T, database interface{})
	}{
		{
			name: "list aliases",
			run: func(t *testing.T) {
				database := newTestDB(t)
				require.NoError(t, database.Create(&models.ServiceAlias{Canonical: "traefik", Alias: "traefik-proxy"}).Error)

				req := httptest.NewRequest(http.MethodGet, "/api/services/aliases", nil)
				rr := httptest.NewRecorder()
				handlers.ListServiceAliases(database)(rr, req)

				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

				var resp []models.ServiceAlias
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
				require.Len(t, resp, 1)
				assert.Equal(t, "traefik", resp[0].Canonical)
				assert.Equal(t, "traefik-proxy", resp[0].Alias)
			},
		},
		{
			name: "create alias",
			run: func(t *testing.T) {
				database := newTestDB(t)

				req := httptest.NewRequest(http.MethodPost, "/api/services/aliases", bytes.NewBufferString(`{"canonical":"traefik","alias":"traefik-proxy"}`))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				handlers.CreateServiceAlias(database)(rr, req)

				require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

				var resp models.ServiceAlias
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
				assert.Equal(t, "traefik", resp.Canonical)
				assert.Equal(t, "traefik-proxy", resp.Alias)

				var alias models.ServiceAlias
				require.NoError(t, database.Where("alias = ?", "traefik-proxy").First(&alias).Error)
				assert.Equal(t, "traefik", alias.Canonical)
			},
		},
		{
			name: "reject duplicate alias",
			run: func(t *testing.T) {
				database := newTestDB(t)
				require.NoError(t, database.Create(&models.ServiceAlias{Canonical: "traefik", Alias: "traefik-proxy"}).Error)

				req := httptest.NewRequest(http.MethodPost, "/api/services/aliases", bytes.NewBufferString(`{"canonical":"traefik","alias":"traefik-proxy"}`))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				handlers.CreateServiceAlias(database)(rr, req)

				assert.Equal(t, http.StatusConflict, rr.Code)
			},
		},
		{
			name: "returns internal error for non-duplicate db failure",
			run: func(t *testing.T) {
				database := newTestDB(t)
				require.NoError(t, database.Exec("DROP TABLE service_aliases").Error)

				req := httptest.NewRequest(http.MethodPost, "/api/services/aliases", bytes.NewBufferString(`{"canonical":"traefik","alias":"traefik-proxy"}`))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				handlers.CreateServiceAlias(database)(rr, req)

				assert.Equal(t, http.StatusInternalServerError, rr.Code)
			},
		},
		{
			name: "delete alias",
			run: func(t *testing.T) {
				database := newTestDB(t)
				require.NoError(t, database.Create(&models.ServiceAlias{Canonical: "traefik", Alias: "traefik-proxy"}).Error)

				router := chi.NewRouter()
				router.Delete("/api/services/aliases/{alias}", handlers.DeleteServiceAlias(database))

				req := httptest.NewRequest(http.MethodDelete, "/api/services/aliases/traefik-proxy", nil)
				rr := httptest.NewRecorder()
				router.ServeHTTP(rr, req)

				assert.Equal(t, http.StatusNoContent, rr.Code)

				var count int64
				require.NoError(t, database.Model(&models.ServiceAlias{}).Where("alias = ?", "traefik-proxy").Count(&count).Error)
				assert.Zero(t, count)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
