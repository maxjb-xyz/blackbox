package queue_test

import (
	"path/filepath"
	"testing"
	"time"

	"blackbox/agent/internal/queue"
	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
)

func newTestQueue(t *testing.T) *queue.Queue {
	t.Helper()
	q, err := queue.New(filepath.Join(t.TempDir(), "queue.db"))
	if err != nil {
		t.Fatalf("queue.New: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })
	return q
}

func TestPush_StoresEntry(t *testing.T) {
	q := newTestQueue(t)

	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "node-1",
		Source:    "docker",
		Service:   "nginx",
		Event:     "start",
		Content:   "Container nginx started",
	}

	if err := q.Push(entry); err != nil {
		t.Fatalf("Push: %v", err)
	}

	rows, err := q.Flush(10)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ID != entry.ID {
		t.Errorf("expected id %q, got %q", entry.ID, rows[0].ID)
	}
	if rows[0].Content != entry.Content {
		t.Errorf("expected content %q, got %q", entry.Content, rows[0].Content)
	}
}

func TestFlush_ReturnsEntriesInFIFOOrder(t *testing.T) {
	q := newTestQueue(t)

	base := time.Now().Add(-10 * time.Minute)
	ids := make([]string, 4)
	for i := range ids {
		ids[i] = ulid.Make().String()
		if err := q.PushAt(types.Entry{ID: ids[i], Source: "docker", Event: "start"}, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("PushAt %d: %v", i, err)
		}
	}

	rows, err := q.Flush(10)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(rows) != len(ids) {
		t.Fatalf("expected %d rows, got %d", len(ids), len(rows))
	}
	for i, row := range rows {
		if row.ID != ids[i] {
			t.Errorf("position %d: expected id %q, got %q", i, ids[i], row.ID)
		}
	}
}

func TestFlush_RespectsLimit(t *testing.T) {
	q := newTestQueue(t)

	for i := 0; i < 5; i++ {
		if err := q.Push(types.Entry{ID: ulid.Make().String(), Source: "docker", Event: "start"}); err != nil {
			t.Fatalf("Push %d: %v", i, err)
		}
	}

	rows, err := q.Flush(3)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(rows))
	}
}

func TestFlush_EmptyQueueReturnsNil(t *testing.T) {
	q := newTestQueue(t)

	rows, err := q.Flush(10)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestDelete_RemovesEntriesByID(t *testing.T) {
	q := newTestQueue(t)

	ids := make([]string, 3)
	for i := range ids {
		ids[i] = ulid.Make().String()
		if err := q.Push(types.Entry{ID: ids[i], Source: "docker", Event: "start"}); err != nil {
			t.Fatalf("Push %d: %v", i, err)
		}
	}

	if err := q.Delete(ids[:2]); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	rows, err := q.Flush(10)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 remaining row, got %d", len(rows))
	}
	if rows[0].ID != ids[2] {
		t.Errorf("expected remaining id %q, got %q", ids[2], rows[0].ID)
	}
}

func TestSweepStale_RemovesOldEntries(t *testing.T) {
	q := newTestQueue(t)

	oldEntry := types.Entry{ID: ulid.Make().String(), Source: "docker", Event: "start"}
	if err := q.PushAt(oldEntry, time.Now().Add(-8*24*time.Hour)); err != nil {
		t.Fatalf("PushAt: %v", err)
	}

	freshEntry := types.Entry{ID: ulid.Make().String(), Source: "docker", Event: "stop"}
	if err := q.Push(freshEntry); err != nil {
		t.Fatalf("Push fresh: %v", err)
	}

	n, err := q.SweepStale(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("SweepStale: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 stale row deleted, got %d", n)
	}

	rows, err := q.Flush(10)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != freshEntry.ID {
		t.Errorf("expected only fresh entry to remain, got %+v", rows)
	}
}
