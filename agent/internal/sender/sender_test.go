package sender

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"blackbox/agent/internal/client"
	"blackbox/agent/internal/queue"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
)

func newTestSender(t *testing.T, srv *httptest.Server) (*Sender, *queue.Queue) {
	t.Helper()
	q, err := queue.New(filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatalf("queue.New: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })
	c := client.New(srv.URL, "test-token", "node-1")
	s := newWithFlushInterval(c, q, 20*time.Millisecond)
	return s, q
}

func TestSender_FlushesQueuedEntries(t *testing.T) {
	var batchReceived atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var entries []types.Entry
		_ = json.NewDecoder(r.Body).Decode(&entries)
		batchReceived.Add(int32(len(entries)))
		ids := make([]string, len(entries))
		for i, e := range entries {
			ids[i] = e.ID
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"accepted": ids,
			"failed":   []interface{}{},
		})
	}))
	defer srv.Close()

	s, _ := newTestSender(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)

	entry := types.Entry{
		ID:      ulid.Make().String(),
		Source:  "docker",
		Service: "nginx",
		Event:   "start",
		Content: "test",
	}
	s.Chan() <- entry

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if batchReceived.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if batchReceived.Load() < 1 {
		t.Fatal("expected entry to be sent within 500ms")
	}
}

func TestSender_LeavesEntriesOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	s, q := newTestSender(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)

	entry := types.Entry{
		ID:      ulid.Make().String(),
		Source:  "docker",
		Service: "nginx",
		Event:   "start",
		Content: "test",
	}
	s.Chan() <- entry

	time.Sleep(200 * time.Millisecond)

	rows, err := q.Flush(10)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected entry to remain in queue after server failure")
	}
}
