package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/server/internal/db"
	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func setupExcludeDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := db.Init(":memory:")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	return database
}

func TestListExcludedTargetsEmpty(t *testing.T) {
	database := setupExcludeDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/excluded-targets", nil)
	rr := httptest.NewRecorder()
	ListExcludedTargets(database)(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var targets []models.ExcludedTarget
	if err := json.NewDecoder(rr.Body).Decode(&targets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("expected empty slice, got %d items", len(targets))
	}
}

func TestCreateExcludedTargetDuplicate(t *testing.T) {
	database := setupExcludeDB(t)
	body := []byte(`{"type":"container","name":"nginx"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/excluded-targets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	CreateExcludedTarget(database)(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/admin/excluded-targets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	CreateExcludedTarget(database)(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rr.Code)
	}
}

func TestDeleteExcludedTarget(t *testing.T) {
	database := setupExcludeDB(t)
	target := models.ExcludedTarget{ID: "01TESTID00001", Type: "stack", Name: "mystack", CreatedAt: time.Now().UTC()}
	if err := database.Create(&target).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", target.ID)
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/excluded-targets/"+target.ID, nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	DeleteExcludedTarget(database)(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}

func TestIsExcludedContainerAndStack(t *testing.T) {
	database := setupExcludeDB(t)
	if err := database.Create(&models.ExcludedTarget{ID: "01EXCL001", Type: "container", Name: "noise-agent", CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatalf("create container: %v", err)
	}
	if err := database.Create(&models.ExcludedTarget{ID: "01EXCL002", Type: "stack", Name: "ci-stack", CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatalf("create stack: %v", err)
	}

	if !isExcluded(database, types.Entry{Source: "docker", Service: "noise-agent", Metadata: "{}"}) {
		t.Fatal("expected container exclusion")
	}
	meta := `{"Actor":{"Attributes":{"com.docker.compose.project":"ci-stack"}}}`
	if !isExcluded(database, types.Entry{Source: "docker", Service: "runner", Metadata: meta}) {
		t.Fatal("expected stack exclusion")
	}
	if isExcluded(database, types.Entry{Source: "files", Service: "noise-agent", Metadata: "{}"}) {
		t.Fatal("non-docker sources must not be excluded")
	}
}
