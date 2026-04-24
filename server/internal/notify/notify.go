package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/models"
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
	discordSender func(ctx context.Context, webhookURL string, inc models.Incident, event string, incURL string, test bool) error = sendDiscord
	slackSender   func(ctx context.Context, webhookURL string, inc models.Incident, event string, incURL string, test bool) error = sendSlack
	ntfySender    func(ctx context.Context, topicURL string, inc models.Incident, event string, incURL string, test bool) error   = sendNtfy
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

	incURL := d.incidentURL(ctx, inc.ID)

	for _, dest := range dests {
		if !destWantsEvent(dest, event) {
			continue
		}

		dest := dest
		go func() {
			sendSem <- struct{}{}
			defer func() { <-sendSem }()

			sendCtx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
			defer cancel()

			if err := sendTo(sendCtx, dest, inc, event, incURL, false); err != nil {
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

	return sendTo(sendCtx, dest, testIncident(), EventIncidentOpenedConfirmed, "", true)
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

func sendTo(ctx context.Context, dest models.NotificationDest, inc models.Incident, event string, incURL string, test bool) error {
	switch dest.Type {
	case "discord":
		return discordSender(ctx, dest.URL, inc, event, incURL, test)
	case "slack":
		return slackSender(ctx, dest.URL, inc, event, incURL, test)
	case "ntfy":
		return ntfySender(ctx, dest.URL, inc, event, incURL, test)
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
