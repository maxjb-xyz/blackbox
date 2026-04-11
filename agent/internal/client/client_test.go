package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackbox/agent/internal/client"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
)

func TestSendBatch_AcceptsAll(t *testing.T) {
	entries := []types.Entry{
		{ID: ulid.Make().String(), Timestamp: time.Now().UTC(), Source: "docker", Service: "nginx", Event: "start", Content: "Container nginx started"},
		{ID: ulid.Make().String(), Timestamp: time.Now().UTC(), Source: "docker", Service: "redis", Event: "stop", Content: "Container redis stopped"},
	}

	var received []types.Entry
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/push/batch" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Blackbox-Agent-Key") != "test-token" {
			t.Errorf("missing or wrong agent key header")
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode body: %v", err)
		}
		ids := make([]string, len(received))
		for i, e := range received {
			ids[i] = e.ID
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"accepted": ids,
			"failed":   []interface{}{},
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, "test-token", "node-1")
	accepted, failed, err := c.SendBatch(context.Background(), entries)
	if err != nil {
		t.Fatalf("SendBatch: %v", err)
	}
	if len(accepted) != 2 {
		t.Errorf("expected 2 accepted, got %d", len(accepted))
	}
	if len(failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(failed))
	}
	for _, e := range received {
		if e.NodeName != "node-1" {
			t.Errorf("expected NodeName=node-1, got %q", e.NodeName)
		}
	}
}

func TestSendBatch_NonTwoXXReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := client.New(srv.URL, "test-token", "node-1")
	_, _, err := c.SendBatch(context.Background(), []types.Entry{
		{ID: ulid.Make().String(), Source: "docker", Event: "start"},
	})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestSendBatch_FourXXReturnsPermanentError(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "rejected", status)
			}))
			defer srv.Close()

			c := client.New(srv.URL, "test-token", "node-1")
			_, _, err := c.SendBatch(context.Background(), []types.Entry{
				{ID: ulid.Make().String(), Source: "docker", Event: "start"},
			})
			if err == nil {
				t.Fatalf("expected error for %d, got nil", status)
			}
			var permErr *client.PermanentError
			if !errors.As(err, &permErr) {
				t.Errorf("expected *client.PermanentError, got %T: %v", err, err)
			}
		})
	}
}

func TestSendBatch_PartialFailure(t *testing.T) {
	goodID := ulid.Make().String()
	badID := ulid.Make().String()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"accepted": []string{goodID},
			"failed":   []map[string]interface{}{{"id": badID, "reason": "entry id is required", "permanent": true}},
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, "test-token", "node-1")
	accepted, failed, err := c.SendBatch(context.Background(), []types.Entry{
		{ID: goodID, Source: "docker", Event: "start"},
		{ID: badID, Source: "docker", Event: "start"},
	})
	if err != nil {
		t.Fatalf("SendBatch: %v", err)
	}
	if len(accepted) != 1 || accepted[0] != goodID {
		t.Errorf("expected accepted=[%s], got %v", goodID, accepted)
	}
	if len(failed) != 1 || failed[0].ID != badID {
		t.Errorf("expected failed=[%s], got %v", badID, failed)
	}
	if !failed[0].Permanent {
		t.Errorf("expected failed[0].Permanent=true")
	}
}
