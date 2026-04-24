package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"blackbox/shared/types"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(Config{BaseURL: srv.URL, APIToken: "user@pam!test=deadbeef"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, srv
}

func TestClient_ListTasks_Success(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != tasksPath {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "PVEAPIToken=") {
			t.Errorf("missing or malformed Authorization header: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []Task{
				{UPID: "UPID:pve01:00001:0:qmstart:100:root@pam:", Node: "pve01", Type: "qmstart", ID: "100", User: "root@pam", StartTime: 1700000000},
			},
		})
	})

	tasks, err := c.ListTasks(context.Background())
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Type != "qmstart" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}

func TestClient_ListTasks_Unauth(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := c.ListTasks(context.Background())
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestNew_ValidatesInput(t *testing.T) {
	if _, err := New(Config{BaseURL: "", APIToken: "x"}); err == nil {
		t.Error("expected error for empty BaseURL")
	}
	if _, err := New(Config{BaseURL: "https://pve", APIToken: ""}); err == nil {
		t.Error("expected error for empty APIToken")
	}
}

// Watcher tests - drive the client via a scripted server and assert on
// the entries emitted.

type pollScript struct {
	responses [][]Task
	idx       int32
}

func (s *pollScript) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		i := atomic.AddInt32(&s.idx, 1) - 1
		if int(i) >= len(s.responses) {
			// Repeat last response forever.
			i = int32(len(s.responses) - 1)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.responses[i]})
	}
}

func drain(ch <-chan types.Entry, n int, within time.Duration) []types.Entry {
	out := make([]types.Entry, 0, n)
	deadline := time.After(within)
	for len(out) < n {
		select {
		case e := <-ch:
			out = append(out, e)
		case <-deadline:
			return out
		}
	}
	return out
}

func TestWatcher_FirstPollDoesNotEmitHistorical(t *testing.T) {
	script := &pollScript{
		responses: [][]Task{
			{{UPID: "UPID:a:1:0:qmstart:100:root@pam:", Type: "qmstart", ID: "100", User: "root@pam", StartTime: 1, EndTime: 2, Status: "OK"}},
			{},
		},
	}
	c, _ := newTestClient(t, script.handler(t))
	w := NewWatcher(c, "pve01", WithPollInterval(5*time.Millisecond))

	ch := make(chan types.Entry, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	go w.Run(ctx, ch)

	entries := drain(ch, 1, 60*time.Millisecond)
	if len(entries) != 0 {
		t.Fatalf("expected zero entries on first poll of finished tasks, got %d: %+v", len(entries), entries)
	}
}

func TestWatcher_EmitsRunningThenDone(t *testing.T) {
	// First poll: no tasks (establishes baseline).
	// Second poll: task in progress -> emit "running".
	// Third poll: same task finished OK -> emit "done" with CorrelatedID.
	upid := "UPID:pve:1:0:qmstart:101:root@pam:"
	script := &pollScript{
		responses: [][]Task{
			{},
			{{UPID: upid, Type: "qmstart", ID: "101", User: "root@pam", Node: "pve", StartTime: 1700000000}},
			{{UPID: upid, Type: "qmstart", ID: "101", User: "root@pam", Node: "pve", StartTime: 1700000000, EndTime: 1700000005, Status: "OK"}},
		},
	}
	c, _ := newTestClient(t, script.handler(t))
	w := NewWatcher(c, "pve01", WithPollInterval(5*time.Millisecond))

	ch := make(chan types.Entry, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go w.Run(ctx, ch)

	entries := drain(ch, 2, 150*time.Millisecond)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Event != "running" || entries[0].Service != "qmstart:101" {
		t.Errorf("first entry: got event=%q service=%q", entries[0].Event, entries[0].Service)
	}
	if entries[1].Event != "done" {
		t.Errorf("second entry: got event=%q", entries[1].Event)
	}
	if entries[1].CorrelatedID != entries[0].ID {
		t.Errorf("second entry CorrelatedID=%q, want %q", entries[1].CorrelatedID, entries[0].ID)
	}
}

func TestWatcher_EmitsFailed(t *testing.T) {
	upid := "UPID:pve:1:0:vzbackup:999:root@pam:"
	script := &pollScript{
		responses: [][]Task{
			{},
			{{UPID: upid, Type: "vzbackup", ID: "999", User: "root@pam", Node: "pve", StartTime: 1, EndTime: 2, Status: "ERROR: backup failed"}},
		},
	}
	c, _ := newTestClient(t, script.handler(t))
	w := NewWatcher(c, "pve01", WithPollInterval(5*time.Millisecond))

	ch := make(chan types.Entry, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	go w.Run(ctx, ch)

	entries := drain(ch, 1, 100*time.Millisecond)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Event != "failed" {
		t.Errorf("got event=%q, want failed", entries[0].Event)
	}
	if !strings.Contains(entries[0].Content, "FAILED") {
		t.Errorf("content should mention FAILED: %q", entries[0].Content)
	}
}

func TestWatcher_NoDuplicateEmits(t *testing.T) {
	// Same task visible across multiple polls with identical state -> emit once.
	upid := "UPID:pve:1:0:qmstart:55:root@pam:"
	runningTask := Task{UPID: upid, Type: "qmstart", ID: "55", User: "root@pam", Node: "pve", StartTime: 1}
	script := &pollScript{
		responses: [][]Task{
			{}, {runningTask}, {runningTask}, {runningTask},
		},
	}
	c, _ := newTestClient(t, script.handler(t))
	w := NewWatcher(c, "pve01", WithPollInterval(5*time.Millisecond))

	ch := make(chan types.Entry, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	go w.Run(ctx, ch)

	entries := drain(ch, 2, 100*time.Millisecond)
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 entry (running), got %d", len(entries))
	}
}

func TestTaskPhase(t *testing.T) {
	cases := []struct {
		name string
		in   Task
		want string
	}{
		{"running", Task{EndTime: 0, Status: ""}, "running"},
		{"done-ok", Task{EndTime: 5, Status: "OK"}, "done"},
		{"done-ok-lower", Task{EndTime: 5, Status: "ok"}, "done"},
		{"failed", Task{EndTime: 5, Status: "ERROR: x"}, "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := taskPhase(tc.in); got != tc.want {
				t.Errorf("taskPhase(%+v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
