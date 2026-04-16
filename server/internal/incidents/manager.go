package incidents

import (
	"context"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"blackbox/server/internal/hub"
	"blackbox/server/internal/models"
	"blackbox/server/internal/notify"
	"blackbox/shared/types"
	"gorm.io/gorm"
)

const managerChannelSize = 256
const defaultReplayCutoff = 10 * time.Minute

// pendingWatchtower tracks a Watchtower update entry waiting to be linked
// to a subsequent container restart.
type pendingWatchtower struct {
	entry    types.Entry
	deadline time.Time
}

// pendingRecovery tracks a recovery signal that may resolve an open
// incident once the service stays healthy for a short window.
type pendingRecovery struct {
	entry    types.Entry
	deadline time.Time
}

// Manager evaluates incoming entries and manages the incident lifecycle.
type Manager struct {
	db              *gorm.DB
	hub             *hub.Hub
	notifier        *notify.Dispatcher
	enricher        *AIEnricher
	replayCutoff    time.Duration
	filteredReplays atomic.Int64

	mu                  sync.Mutex
	openIncidents       map[string]string            // "service|node" -> incidentID
	pendingWT           map[string]pendingWatchtower // normalizedService -> pending (watchtower has no node)
	pendingRecover      map[string]pendingRecovery   // "service|node" -> recovery event waiting for stability
	recentDies          map[string][]time.Time       // "service|node" -> die timestamps
	recentSystemdEvents map[string][]time.Time       // "service|node" -> failed/restart timestamps
}

// incidentKey returns the composite lookup key for open incidents and recent-die
// tracking. node may be empty for webhook-sourced events that carry no node info.
func incidentKey(svc, node string) string {
	return svc + "|" + node
}

// NewManager creates a Manager. Call Run in a goroutine.
func NewManager(db *gorm.DB, h *hub.Hub, notifier *notify.Dispatcher) *Manager {
	cutoff := defaultReplayCutoff
	if v := os.Getenv("INCIDENT_REPLAY_CUTOFF"); v != "" {
		if d, err := time.ParseDuration(v); err != nil {
			log.Printf("incidents: invalid INCIDENT_REPLAY_CUTOFF %q: %v; using default %v", v, err, cutoff)
		} else if d < 0 {
			log.Printf("incidents: negative INCIDENT_REPLAY_CUTOFF %v ignored; using default %v", d, cutoff)
		} else {
			cutoff = d
		}
	}
	m := &Manager{
		db:                  db,
		hub:                 h,
		notifier:            notifier,
		replayCutoff:        cutoff,
		openIncidents:       make(map[string]string),
		pendingWT:           make(map[string]pendingWatchtower),
		pendingRecover:      make(map[string]pendingRecovery),
		recentDies:          make(map[string][]time.Time),
		recentSystemdEvents: make(map[string][]time.Time),
	}
	m.enricher = NewAIEnricher(db, m.broadcastUpdated)
	return m
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

	m.mu.Lock()
	m.rebuildOpenIncidentsLocked()
	m.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			if m.replayCutoff > 0 && time.Since(entry.Timestamp) > m.replayCutoff {
				m.filteredReplays.Add(1)
				continue
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

func (m *Manager) rebuildOpenIncidentsLocked() {
	m.openIncidents = make(map[string]string)

	var incidents []models.Incident
	if err := m.db.Where("status = ?", "open").Order("opened_at ASC, id ASC").Find(&incidents).Error; err != nil {
		return
	}

	for _, inc := range incidents {
		services := uniqueStrings(parseJSONStringSlice(inc.Services))
		if len(services) == 0 {
			continue
		}
		nodes := preferNonWebhookValues(parseJSONStringSlice(inc.NodeNames))
		for _, service := range services {
			m.registerOpenIncidentKeys(inc.ID, service, nodes)
		}
	}
}
