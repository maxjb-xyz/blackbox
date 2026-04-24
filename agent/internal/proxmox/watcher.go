package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"blackbox/shared/types"
)

const (
	defaultPollInterval = 10 * time.Second
	backoffMax          = 5 * time.Minute

	sourceProxmox = "proxmox"

	phaseRunning = "running"
	phaseDone    = "done"
	phaseFailed  = "failed"
)

// Watcher polls a Proxmox cluster and emits Entry records for task
// state transitions. It deduplicates by UPID so a task in progress is
// only announced once for its running phase and once when it finishes.
type Watcher struct {
	client       *Client
	nodeName     string
	pollInterval time.Duration

	// seen maps upid -> last emitted phase so pollOnce only emits on transitions.
	seen map[string]string
	// On first poll we record existing tasks without emitting them - avoids
	// re-announcing historical events every time the agent restarts.
	firstPoll bool
	// upidToStartID links a completion entry back to its running entry
	// via Entry.CorrelatedID.
	upidToStartID map[string]string
}

// WatcherOption tweaks Watcher construction.
type WatcherOption func(*Watcher)

// WithPollInterval overrides the default 10s polling cadence. Intended
// mainly for tests.
func WithPollInterval(d time.Duration) WatcherOption {
	return func(w *Watcher) {
		if d > 0 {
			w.pollInterval = d
		}
	}
}

// NewWatcher builds a Watcher. client must be non-nil.
func NewWatcher(client *Client, nodeName string, opts ...WatcherOption) *Watcher {
	w := &Watcher{
		client:        client,
		nodeName:      nodeName,
		pollInterval:  defaultPollInterval,
		seen:          make(map[string]string),
		firstPoll:     true,
		upidToStartID: make(map[string]string),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Run polls the Proxmox API until ctx is cancelled. It logs transient
// errors and applies exponential backoff so a flaky network or
// restarted Proxmox host does not busy-loop.
func (w *Watcher) Run(ctx context.Context, out chan<- types.Entry) {
	backoff := w.pollInterval
	for {
		if err := w.pollOnce(ctx, out); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("proxmox watcher: %v - retry in %s", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > backoffMax {
				backoff = backoffMax
			}
			continue
		}
		backoff = w.pollInterval
		select {
		case <-ctx.Done():
			return
		case <-time.After(w.pollInterval):
		}
	}
}

func (w *Watcher) pollOnce(ctx context.Context, out chan<- types.Entry) error {
	tasks, err := w.client.ListTasks(ctx)
	if err != nil {
		return err
	}

	currentUPIDs := make(map[string]struct{}, len(tasks))
	for _, t := range tasks {
		currentUPIDs[t.UPID] = struct{}{}
		entry, ok := w.observe(t)
		if !ok {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- entry:
		}
	}

	for upid := range w.seen {
		if _, ok := currentUPIDs[upid]; !ok {
			delete(w.seen, upid)
			delete(w.upidToStartID, upid)
		}
	}

	w.firstPoll = false
	return nil
}

// observe returns the Entry (if any) to emit for the given task given
// prior watcher state. It mutates w.seen / w.upidToStartID.
func (w *Watcher) observe(t Task) (types.Entry, bool) {
	if t.UPID == "" {
		return types.Entry{}, false
	}

	phase := taskPhase(t)

	if w.firstPoll {
		w.seen[t.UPID] = phase
		return types.Entry{}, false
	}

	if prev, known := w.seen[t.UPID]; known && prev == phase {
		return types.Entry{}, false
	}

	entry := w.buildEntry(t, phase)

	if phase == phaseRunning {
		w.upidToStartID[t.UPID] = entry.ID
	} else if startID, ok := w.upidToStartID[t.UPID]; ok {
		entry.CorrelatedID = startID
		delete(w.upidToStartID, t.UPID)
	}

	w.seen[t.UPID] = phase
	return entry, true
}

func taskPhase(t Task) string {
	switch {
	case t.EndTime == 0 && t.Status == "":
		return phaseRunning
	case strings.EqualFold(strings.TrimSpace(t.Status), "OK"):
		return phaseDone
	default:
		return phaseFailed
	}
}

// taskMetadata is the JSON shape emitted in Entry.Metadata. Kept as a
// typed struct so json.Marshal avoids the map allocation + boxing path.
type taskMetadata struct {
	UPID      string `json:"upid"`
	Node      string `json:"node"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	User      string `json:"user"`
	Status    string `json:"status,omitempty"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time,omitempty"`
	PID       int    `json:"pid,omitempty"`
}

func (w *Watcher) buildEntry(t Task, phase string) types.Entry {
	ts := time.Unix(t.StartTime, 0).UTC()
	if phase != phaseRunning && t.EndTime > 0 {
		ts = time.Unix(t.EndTime, 0).UTC()
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	service := t.Type
	if t.ID != "" {
		service = t.Type + ":" + t.ID
	}

	meta, _ := json.Marshal(taskMetadata{
		UPID:      t.UPID,
		Node:      t.Node,
		Type:      t.Type,
		ID:        t.ID,
		User:      t.User,
		Status:    t.Status,
		StartTime: t.StartTime,
		EndTime:   t.EndTime,
		PID:       t.PID,
	})

	return types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: ts,
		NodeName:  w.nodeName,
		Source:    sourceProxmox,
		Service:   service,
		Event:     phase,
		Content:   buildContent(t, phase),
		Metadata:  string(meta),
	}
}

func buildContent(t Task, phase string) string {
	who := t.User
	if who == "" {
		who = "unknown"
	}
	target := t.ID
	if target == "" {
		target = "cluster"
	}
	switch phase {
	case phaseRunning:
		return fmt.Sprintf("Proxmox %s on %s started by %s (node %s)", t.Type, target, who, t.Node)
	case phaseDone:
		return fmt.Sprintf("Proxmox %s on %s completed OK (node %s)", t.Type, target, t.Node)
	case phaseFailed:
		status := strings.TrimSpace(t.Status)
		if status == "" {
			status = "ERROR"
		}
		return fmt.Sprintf("Proxmox %s on %s FAILED: %s (node %s)", t.Type, target, status, t.Node)
	default:
		return fmt.Sprintf("Proxmox %s on %s (%s)", t.Type, target, phase)
	}
}
