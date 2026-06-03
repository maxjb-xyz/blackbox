package notify

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

const flushInterval = time.Minute

// Flusher emits a digest of held notifications when a destination's quiet
// window ends.
type Flusher struct {
	db  *gorm.DB
	now func() time.Time
}

func NewFlusher(db *gorm.DB) *Flusher {
	return &Flusher{db: db, now: time.Now}
}

// Run flushes once immediately (to catch rows held across a restart) then on a
// ticker until ctx is cancelled.
func (f *Flusher) Run(ctx context.Context) {
	if err := f.flushAll(ctx); err != nil {
		log.Printf("notify: initial digest flush: %v", err)
	}
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := f.flushAll(ctx); err != nil {
				log.Printf("notify: digest flush: %v", err)
			}
		}
	}
}

// flushAll sends a digest for every defer-mode destination that is currently
// outside its quiet window and has pending held rows.
func (f *Flusher) flushAll(ctx context.Context) error {
	var dests []models.NotificationDest
	if err := f.db.Where("enabled = ? AND quiet_hours_enabled = ? AND quiet_hours_mode = ?", true, true, "defer").
		Find(&dests).Error; err != nil {
		return err
	}
	now := f.now()
	for _, dest := range dests {
		if inQuietWindow(now, dest.QuietHoursStart, dest.QuietHoursEnd) {
			continue
		}
		if err := f.flushDest(ctx, dest, now); err != nil {
			log.Printf("notify: flush dest %q: %v", dest.Name, err)
		}
	}
	return nil
}

func (f *Flusher) flushDest(ctx context.Context, dest models.NotificationDest, now time.Time) error {
	var held []models.NotificationLog
	if err := f.db.Where("dest_id = ? AND decision = ? AND flushed_at IS NULL", dest.ID, decisionHeld).
		Order("created_at ASC").Find(&held).Error; err != nil {
		return err
	}
	if len(held) == 0 {
		return nil
	}

	ids := make([]string, 0, len(held))
	for _, h := range held {
		ids = append(ids, h.IncidentID)
	}
	var incidents []models.Incident
	if err := f.db.Where("id IN ?", ids).Find(&incidents).Error; err != nil {
		return err
	}

	if err := f.sendDigest(ctx, dest, incidents); err != nil {
		return err
	}

	if err := f.db.Model(&models.NotificationLog{}).
		Where("dest_id = ? AND decision = ? AND flushed_at IS NULL", dest.ID, decisionHeld).
		Update("flushed_at", now).Error; err != nil {
		return err
	}
	return f.db.Create(&models.NotificationLog{
		ID: ulid.Make().String(), DestID: dest.ID, Decision: decisionDigest,
		Note: fmt.Sprintf("digest of %d held", len(held)), CreatedAt: now,
	}).Error
}

// sendDigest renders one rollup message and posts it via the destination's
// channel. It bypasses the rate limit (already a single message).
func (f *Flusher) sendDigest(ctx context.Context, dest models.NotificationDest, incidents []models.Incident) error {
	title := fmt.Sprintf("%d notifications held during quiet hours (%s–%s)",
		len(incidents), dest.QuietHoursStart, dest.QuietHoursEnd)

	var lines []string
	for _, inc := range incidents {
		conf := inc.Confidence
		if conf == "" {
			conf = "unknown"
		}
		lines = append(lines, fmt.Sprintf("• [%s] %s", conf, inc.Title))
	}
	body := title + "\n" + strings.Join(lines, "\n")

	sendCtx, cancel := context.WithTimeout(ctx, notifyTimeout)
	defer cancel()

	digestInc := models.Incident{
		ID: "digest", Title: title, Services: "[]", NodeNames: "[]", Metadata: "{}",
		OpenedAt: f.now(), Confidence: "confirmed", Status: "open",
	}
	// Reuse the provider senders, passing the rollup as the suppressed note so
	// the per-incident body stays the digest title and the lines append below.
	return sendTo(sendCtx, dest, digestInc, EventIncidentOpenedConfirmed, "", body, false)
}
