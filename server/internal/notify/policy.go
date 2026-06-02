package notify

import (
	"fmt"
	"time"

	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

// logDecision inserts a NotificationLog row. createdAt is explicit so the gate
// and tests control time via the dispatcher clock.
func (d *Dispatcher) logDecision(db *gorm.DB, destID, incidentID, event, decision, note string, createdAt time.Time) error {
	return db.Create(&models.NotificationLog{
		ID:         ulid.Make().String(),
		DestID:     destID,
		IncidentID: incidentID,
		Event:      event,
		Decision:   decision,
		Note:       note,
		CreatedAt:  createdAt,
	}).Error
}

// sentCountSince counts sent rows for a destination at or after `since`.
func (d *Dispatcher) sentCountSince(db *gorm.DB, destID string, since time.Time) (int64, error) {
	var n int64
	err := db.Model(&models.NotificationLog{}).
		Where("dest_id = ? AND decision = ? AND created_at >= ?", destID, decisionSent, since).
		Count(&n).Error
	return n, err
}

// droppedRateSinceLastSent counts dropped_rate rows recorded after the most
// recent sent row for a destination (or all of them if it never sent).
func (d *Dispatcher) droppedRateSinceLastSent(db *gorm.DB, destID string) (int64, error) {
	var last models.NotificationLog
	err := db.Where("dest_id = ? AND decision = ?", destID, decisionSent).
		Order("created_at DESC").First(&last).Error

	q := db.Model(&models.NotificationLog{}).
		Where("dest_id = ? AND decision = ?", destID, decisionDroppedRate)
	if err == nil {
		q = q.Where("created_at > ?", last.CreatedAt)
	} else if err != gorm.ErrRecordNotFound {
		return 0, err
	}

	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// decide evaluates quiet hours then the rate limit for a destination and
// returns the decision plus any suppressed-count note to append on a send. It
// performs reads only; the caller writes the resulting log row and sends.
func (d *Dispatcher) decide(db *gorm.DB, dest models.NotificationDest) (decision, note string, err error) {
	now := d.now()

	if dest.QuietHoursEnabled && inQuietWindow(now, dest.QuietHoursStart, dest.QuietHoursEnd) {
		if dest.QuietHoursMode == "defer" {
			return decisionHeld, "", nil
		}
		return decisionDroppedQuiet, "", nil
	}

	if dest.RateLimitEnabled && dest.RateLimitCount > 0 {
		window := rateWindow(dest.RateLimitUnit)
		count, err := d.sentCountSince(db, dest.ID, now.Add(-window))
		if err != nil {
			return "", "", err
		}
		if count >= int64(dest.RateLimitCount) {
			return decisionDroppedRate, "", nil
		}
		suppressed, err := d.droppedRateSinceLastSent(db, dest.ID)
		if err != nil {
			return "", "", err
		}
		if suppressed > 0 {
			note = fmt.Sprintf("(+%d suppressed in the last %s)", suppressed, dest.RateLimitUnit)
		}
	}

	return decisionSent, note, nil
}
