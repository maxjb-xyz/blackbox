package models

import "time"

type NotificationDest struct {
	ID      string `gorm:"primaryKey" json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Events  string `json:"events"`
	Enabled bool   `json:"enabled"`

	// Quiet hours (server timezone). Window may wrap midnight.
	QuietHoursEnabled bool   `json:"quiet_hours_enabled"`
	QuietHoursStart   string `json:"quiet_hours_start"` // "HH:MM"
	QuietHoursEnd     string `json:"quiet_hours_end"`   // "HH:MM"
	QuietHoursMode    string `json:"quiet_hours_mode"`  // "drop" | "defer"

	// Rate limit: at most RateLimitCount sends per RateLimitUnit (sliding window).
	RateLimitEnabled bool   `json:"rate_limit_enabled"`
	RateLimitCount   int    `json:"rate_limit_count"`
	RateLimitUnit    string `json:"rate_limit_unit"` // "hour" | "day"

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
