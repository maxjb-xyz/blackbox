package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"blackbox/server/internal/models"
	"gorm.io/gorm"
)

const notifyTimeout = 10 * time.Second

// Shared HTTP client used by all provider send functions.
var httpClient = &http.Client{Timeout: notifyTimeout}

const (
	EventIncidentOpenedConfirmed = "incident_opened_confirmed"
	EventIncidentOpenedSuspected = "incident_opened_suspected"
	EventIncidentConfirmed       = "incident_confirmed"
	EventIncidentResolved        = "incident_resolved"
)

var (
	discordSender = sendDiscord
	slackSender   = sendSlack
	ntfySender    = sendNtfy
)

// Dispatcher fans out incident events to enabled notification destinations.
type Dispatcher struct {
	db *gorm.DB
}

// NewDispatcher creates a Dispatcher backed by the given database.
func NewDispatcher(db *gorm.DB) *Dispatcher {
	return &Dispatcher{db: db}
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

	for _, dest := range dests {
		if !destWantsEvent(dest, event) {
			continue
		}

		dest := dest
		go func() {
			sendCtx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
			defer cancel()

			if err := sendTo(sendCtx, dest, inc, false); err != nil {
				log.Printf("notify: send to %q (%s): %v", dest.Name, dest.Type, err)
			}
		}()
	}
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

	return sendTo(sendCtx, dest, testIncident(), true)
}

func destWantsEvent(dest models.NotificationDest, event string) bool {
	var events []string
	if err := json.Unmarshal([]byte(dest.Events), &events); err != nil {
		return false
	}

	for _, candidate := range events {
		if candidate == event {
			return true
		}
	}

	return false
}

func sendTo(ctx context.Context, dest models.NotificationDest, inc models.Incident, test bool) error {
	switch dest.Type {
	case "discord":
		return discordSender(ctx, dest.URL, inc, test)
	case "slack":
		return slackSender(ctx, dest.URL, inc, test)
	case "ntfy":
		return ntfySender(ctx, dest.URL, inc, test)
	default:
		return fmt.Errorf("unknown destination type: %s", dest.Type)
	}
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
