package systemd

import (
	"context"
	"log"
	"sync"
	"time"

	"blackbox/shared/types"
)

// Settings holds the dynamically refreshed list of systemd units to watch.
type Settings struct {
	mu    sync.RWMutex
	units []string
}

// NewSettings creates a Settings with an initial unit list.
func NewSettings(units []string) *Settings {
	s := &Settings{}
	cp := make([]string, len(units))
	copy(cp, units)
	s.units = cp
	return s
}

// Units returns a snapshot of the current unit list.
func (s *Settings) Units() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]string, len(s.units))
	copy(cp, s.units)
	return cp
}

// SetUnits replaces the unit list atomically.
func (s *Settings) SetUnits(units []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]string, len(units))
	copy(cp, units)
	s.units = cp
}

// Watch opens the systemd journal and emits lifecycle and OOM kill entries.
// Reconnects on error with 5s backoff.
func Watch(ctx context.Context, nodeName string, settings *Settings, out chan<- types.Entry) {
	defer closeCachedJournal()

	for {
		if err := watch(ctx, nodeName, settings, out); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("systemd watcher: error: %v - reconnecting in 5s", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		} else {
			return
		}
	}
}

// mapTransition returns the event name for a unit ActiveState transition,
// or "" if no entry should be emitted.
func mapTransition(prev, curr string) string {
	switch {
	case curr == "failed":
		return "failed"
	case prev == "failed" && curr == "activating":
		return "restart"
	case curr == "active" && (prev == "activating" || prev == "inactive" || prev == "failed"):
		return "started"
	case curr == "deactivating" && prev == "active":
		return "stopped"
	case curr == "inactive" && prev == "deactivating":
		return "stopped"
	}
	return ""
}
