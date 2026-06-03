package models

import "time"

// NotificationLog records every per-destination send decision made by the
// dispatcher. It powers sliding-window rate counting, the suppressed-count
// note, the deferred-digest hold queue, and "why was/wasn't I paged?"
// observability.
type NotificationLog struct {
	ID         string `gorm:"primaryKey" json:"id"`
	DestID     string `gorm:"index:idx_notiflog_dest_created;index:idx_notiflog_dest_decision" json:"dest_id"`
	IncidentID string `json:"incident_id"`
	Event      string `json:"event"`
	// Decision is one of: sent, held, dropped_rate, dropped_quiet, failed, digest.
	Decision  string    `gorm:"index:idx_notiflog_dest_decision" json:"decision"`
	Note      string    `json:"note"`
	CreatedAt time.Time `gorm:"index:idx_notiflog_dest_created" json:"created_at"`
	// FlushedAt is set on held rows when they are rolled into a digest.
	FlushedAt *time.Time `gorm:"index:idx_notiflog_dest_decision" json:"flushed_at,omitempty"`
}
