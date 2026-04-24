package models

import "time"

// Playbook is an admin-curated markdown runbook that Blackbox attaches to
// incidents whose services match ServicePattern. Patterns use shell-glob
// semantics via path.Match (e.g. "docker:nextcloud", "qmstart:*", "*:nextcloud").
//
// When an incident is opened the server collects every enabled playbook
// whose pattern matches any of the incident's services, sorted by Priority
// descending and then Name ascending.
type Playbook struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	Name           string    `gorm:"index" json:"name"`
	ServicePattern string    `json:"service_pattern"`
	Priority       int       `json:"priority"`
	ContentMD      string    `json:"content_md"`
	Enabled        bool      `gorm:"index" json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
