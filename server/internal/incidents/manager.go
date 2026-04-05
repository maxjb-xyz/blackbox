package incidents

import (
	"context"
	"sync"
	"time"

	"blackbox/server/internal/hub"
	"blackbox/shared/types"
	"gorm.io/gorm"
)

const managerChannelSize = 256

// pendingWatchtower tracks a Watchtower update entry waiting to be linked
// to a subsequent container restart.
type pendingWatchtower struct {
	entry    types.Entry
	deadline time.Time
}

// Manager evaluates incoming entries and manages the incident lifecycle.
type Manager struct {
	db  *gorm.DB
	hub *hub.Hub

	mu            sync.Mutex
	openIncidents map[string]string            // normalizedService -> incidentID
	pendingWT     map[string]pendingWatchtower // normalizedService -> pending
	recentDies    map[string][]time.Time       // normalizedService -> die timestamps
}

// NewManager creates a Manager. Call Run in a goroutine.
func NewManager(db *gorm.DB, h *hub.Hub) *Manager {
	return &Manager{
		db:            db,
		hub:           h,
		openIncidents: make(map[string]string),
		pendingWT:     make(map[string]pendingWatchtower),
		recentDies:    make(map[string][]time.Time),
	}
}

// NewChannel returns a buffered channel sized for the Manager.
// Pass the send-only end to handlers; pass the receive end to Run.
func NewChannel() chan types.Entry {
	return make(chan types.Entry, managerChannelSize)
}

// Run processes entries from ch until ctx is cancelled.
func (m *Manager) Run(ctx context.Context, ch <-chan types.Entry) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			m.processEntry(entry)
		case <-ticker.C:
			m.sweepExpiredSuspected()
		}
	}
}

func (m *Manager) sweepExpiredSuspected() {
	// Implemented in rules.go
	m.mu.Lock()
	defer m.mu.Unlock()
	sweepExpiredSuspectedLocked(m)
}
