package correlation

import (
	"errors"
	"time"

	"blackbox/shared/types"
	"gorm.io/gorm"
)

const correlationWindow = 120 * time.Second

// FindCause returns the most recent non-webhook entry for the given service
// within the correlation window ending at the supplied timestamp.
func FindCause(db *gorm.DB, service string, at time.Time) (*types.Entry, error) {
	windowStart := at.Add(-correlationWindow)

	var cause types.Entry
	err := db.Where(
		"service = ? AND timestamp BETWEEN ? AND ? AND source != ?",
		service, windowStart, at, "webhook",
	).Order("timestamp DESC").First(&cause).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &cause, nil
}
