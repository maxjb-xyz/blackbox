package systemd

import (
	"context"
	"log"
	"slices"
	"strings"
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

// HasUnit reports whether unit is currently configured for watching.
func (s *Settings) HasUnit(unit string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Contains(s.units, unit)
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

type journalMatcher interface {
	FlushMatches()
	AddMatch(string) error
	AddDisjunction() error
}

func rebuildJournalMatches(j journalMatcher, units []string) error {
	j.FlushMatches()

	groups := make([][]string, 0, len(units)*2+1)
	for _, unit := range units {
		unit = strings.TrimSpace(unit)
		if unit == "" {
			continue
		}
		// Only match manager-originated lifecycle messages. _SYSTEMD_UNIT= is
		// too broad – it includes every log line emitted by the service itself.
		groups = append(groups,
			[]string{"UNIT=" + unit, "_PID=1"},
			[]string{"OBJECT_SYSTEMD_UNIT=" + unit, "_UID=0"},
		)
	}

	// Keep kernel OOM monitoring independent of the configured unit list.
	groups = append(groups, []string{"SYSLOG_FACILITY=0"})

	return applyMatchGroups(j, groups)
}

func applyMatchGroups(j journalMatcher, groups [][]string) error {
	firstGroup := true
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		if !firstGroup {
			if err := j.AddDisjunction(); err != nil {
				return err
			}
		}
		firstGroup = false
		for _, match := range group {
			if err := j.AddMatch(match); err != nil {
				return err
			}
		}
	}
	return nil
}

func classifyLifecycleEvent(fields map[string]string, watchedUnits []string) (string, string) {
	unit := trackedUnit(fields, watchedUnits)
	if unit == "" {
		return "", ""
	}
	if !isManagerLifecycleEntry(fields) {
		return "", ""
	}

	message := strings.TrimSpace(fields["MESSAGE"])
	if message == "" {
		return "", ""
	}

	switch {
	case strings.Contains(message, "Scheduled restart job"):
		return unit, "restart"
	case strings.Contains(message, "Failed with result"):
		return unit, "failed"
	case strings.HasPrefix(message, "Failed to start "):
		return unit, "failed"
	case strings.HasPrefix(message, "Started "):
		return unit, "started"
	case strings.HasPrefix(message, "Stopped "):
		return unit, "stopped"
	}

	return "", ""
}

func isManagerLifecycleEntry(fields map[string]string) bool {
	return strings.TrimSpace(fields["UNIT"]) != "" ||
		strings.TrimSpace(fields["OBJECT_SYSTEMD_UNIT"]) != "" ||
		strings.TrimSpace(fields["_PID"]) == "1"
}

func trackedUnit(fields map[string]string, watchedUnits []string) string {
	for _, key := range []string{"UNIT", "OBJECT_SYSTEMD_UNIT", "COREDUMP_UNIT", "_SYSTEMD_UNIT"} {
		unit := strings.TrimSpace(fields[key])
		if unit == "" {
			continue
		}
		if slices.Contains(watchedUnits, unit) {
			return unit
		}
	}
	return ""
}
