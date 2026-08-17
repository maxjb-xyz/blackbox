package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

const notifyTimeout = 10 * time.Second
const maxConcurrentSends = 8
const baseURLKey = "base_url"
const maxSlackAIAnalysisChars = 2000

// Shared HTTP client used by all provider send functions.
var httpClient = &http.Client{Timeout: notifyTimeout}

// sendSem limits the number of concurrent outbound notification requests.
var sendSem = make(chan struct{}, maxConcurrentSends)

const (
	EventIncidentOpenedConfirmed = "incident_opened_confirmed"
	EventIncidentOpenedSuspected = "incident_opened_suspected"
	EventIncidentConfirmed       = "incident_confirmed"
	EventIncidentResolved        = "incident_resolved"
	EventAIReviewGenerated       = "incident_ai_review_generated"
)

var (
	discordSender func(ctx context.Context, webhookURL string, inc models.Incident, event, incURL, note string, test bool) error = sendDiscord
	slackSender   func(ctx context.Context, webhookURL string, inc models.Incident, event, incURL, note string, test bool) error = sendSlack
	ntfySender    func(ctx context.Context, topicURL string, inc models.Incident, event, incURL, note string, test bool) error   = sendNtfy
)

// Dispatcher fans out incident events to enabled notification destinations.
type Dispatcher struct {
	db  *gorm.DB
	now func() time.Time
	// destLocks serializes the gate decision + rate-slot reservation per
	// destination so concurrent sends cannot both pass the rate cap.
	destLocks sync.Map // dest.ID -> *sync.Mutex
}

// NewDispatcher creates a Dispatcher backed by the given database.
func NewDispatcher(db *gorm.DB) *Dispatcher {
	return &Dispatcher{db: db, now: time.Now}
}

// destLock returns the per-destination mutex, creating it on first use.
func (d *Dispatcher) destLock(id string) *sync.Mutex {
	mu, _ := d.destLocks.LoadOrStore(id, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// Send loads enabled destinations, filters them by event subscription, and
// dispatches notifications concurrently. Errors are logged and not returned.
func (d *Dispatcher) Send(ctx context.Context, event string, inc models.Incident) {
	query := d.db
	if ctx != nil {
		query = query.WithContext(ctx)
	}

	var dests []models.NotificationDest
	if err := query.Where("enabled = ?", true).Find(&dests).Error; err != nil {
		log.Printf("notify: load destinations: %v", err)
		return
	}

	incURL := d.incidentURL(ctx, inc.ID)

	for _, dest := range dests {
		if !destWantsEvent(dest, event) {
			continue
		}

		dest := dest
		go func() {
			sendSem <- struct{}{}
			defer func() { <-sendSem }()

			d.evaluateAndSend(dest, inc, event, incURL)
		}()
	}
}

// evaluateAndSend gates a single destination, records the decision, and sends
// when allowed. Errors are logged, never returned (fire-and-forget).
func (d *Dispatcher) evaluateAndSend(dest models.NotificationDest, inc models.Incident, event, incURL string) {
	// Gate the destination and, for an allowed send, reserve the rate slot under
	// the per-destination lock. Holding the lock across decide + the sent-row
	// insert closes the read-then-write race where two concurrent evaluations
	// could both pass the cap. HTTP delivery happens outside the lock so a slow
	// destination cannot stall others.
	mu := d.destLock(dest.ID)
	mu.Lock()

	decision, note, err := d.decide(d.db, dest)
	if err != nil {
		mu.Unlock()
		// Fail open: a transient counting-query error must not silently swallow
		// a notification. Deliver best-effort and record the outcome.
		log.Printf("notify: decide for %q: %v (sending anyway)", dest.Name, err)
		d.deliver(dest, inc, event, incURL, "")
		return
	}

	if decision != decisionSent {
		if err := d.logDecision(d.db, dest.ID, inc.ID, event, decision, "", d.now()); err != nil {
			log.Printf("notify: log %s for %q: %v", decision, dest.Name, err)
		}
		mu.Unlock()
		return
	}

	// Reserve the slot by writing the sent row now; it counts toward the cap
	// immediately. If delivery fails we downgrade it to "failed" below so it no
	// longer counts.
	row := &models.NotificationLog{
		ID:         ulid.Make().String(),
		DestID:     dest.ID,
		IncidentID: inc.ID,
		Event:      event,
		Decision:   decisionSent,
		Note:       note,
		CreatedAt:  d.now(),
	}
	if err := d.db.Create(row).Error; err != nil {
		mu.Unlock()
		// Fail open: if the reservation write fails, rate enforcement degrades
		// but the notification must still be delivered.
		log.Printf("notify: reserve sent for %q: %v (sending anyway)", dest.Name, err)
		d.deliver(dest, inc, event, incURL, note)
		return
	}
	mu.Unlock()

	sendCtx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()

	if err := sendTo(sendCtx, dest, inc, event, incURL, note, false); err != nil {
		log.Printf("notify: send to %q (%s): %v", dest.Name, dest.Type, err)
		if uerr := d.db.Model(&models.NotificationLog{}).Where("id = ?", row.ID).
			Updates(map[string]interface{}{"decision": decisionFailed, "note": failedDeliveryNote(err)}).Error; uerr != nil {
			log.Printf("notify: mark failed for %q: %v", dest.Name, uerr)
		}
	}
}

// deliver sends without rate reservation and records the outcome. Used for the
// fail-open path when the gate decision could not be computed.
func (d *Dispatcher) deliver(dest models.NotificationDest, inc models.Incident, event, incURL, note string) {
	sendCtx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()

	decision := decisionSent
	logNote := note
	if err := sendTo(sendCtx, dest, inc, event, incURL, note, false); err != nil {
		log.Printf("notify: send to %q (%s): %v", dest.Name, dest.Type, err)
		decision, logNote = decisionFailed, failedDeliveryNote(err)
	}
	if err := d.logDecision(d.db, dest.ID, inc.ID, event, decision, logNote, d.now()); err != nil {
		log.Printf("notify: log %s for %q: %v", decision, dest.Name, err)
	}
}

func failedDeliveryNote(err error) string {
	if err == nil {
		return ""
	}
	return "delivery failed"
}

// SendTest sends a synthetic payload to a single destination and returns any
// delivery error directly to the caller.
func (d *Dispatcher) SendTest(ctx context.Context, dest models.NotificationDest) error {
	sendCtx := ctx
	if sendCtx == nil {
		sendCtx = context.Background()
	}

	sendCtx, cancel := context.WithTimeout(sendCtx, notifyTimeout)
	defer cancel()

	return sendTo(sendCtx, dest, testIncident(), EventIncidentOpenedConfirmed, "", "", true)
}

func (d *Dispatcher) incidentURL(ctx context.Context, incidentID string) string {
	query := d.db
	if ctx != nil {
		query = query.WithContext(ctx)
	}
	var setting models.AppSetting
	if err := query.First(&setting, "key = ?", baseURLKey).Error; err != nil {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(setting.Value), "/")
	if base == "" {
		return ""
	}
	return base + "/incidents/" + incidentID
}

func destWantsEvent(dest models.NotificationDest, event string) bool {
	var events []string
	if err := json.Unmarshal([]byte(dest.Events), &events); err != nil {
		log.Printf("notify: failed to parse events for destination %q (%s): %v", dest.Name, dest.ID, err)
		return false
	}

	for _, candidate := range events {
		if candidate == event {
			return true
		}
	}

	return false
}

func sendTo(ctx context.Context, dest models.NotificationDest, inc models.Incident, event, incURL, note string, test bool) error {
	switch dest.Type {
	case "discord":
		return discordSender(ctx, dest.URL, inc, event, incURL, note, test)
	case "slack":
		return slackSender(ctx, dest.URL, inc, event, incURL, note, test)
	case "ntfy":
		return ntfySender(ctx, dest.URL, inc, event, incURL, note, test)
	default:
		return fmt.Errorf("unknown destination type: %s", dest.Type)
	}
}

func extractAIAnalysis(inc models.Incident) string {
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(inc.Metadata), &meta); err != nil {
		return ""
	}
	if v, ok := meta["ai_analysis"].(string); ok {
		return v
	}
	return ""
}

func truncateAIAnalysis(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func testIncident() models.Incident {
	return models.Incident{
		ID:         "test",
		Status:     "open",
		Confidence: "confirmed",
		Title:      "Test notification from Blackbox",
		Services:   `["test-service"]`,
		NodeNames:  `["test-node"]`,
		OpenedAt:   time.Now(),
		Metadata:   "{}",
	}
}
