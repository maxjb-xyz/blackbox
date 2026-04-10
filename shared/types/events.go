package types

import "time"

type Entry struct {
	ID             string    `json:"id" gorm:"primaryKey;index:idx_entries_timestamp_id,priority:2"`
	Timestamp      time.Time `json:"timestamp" gorm:"index:idx_entries_timestamp_id,priority:1"`
	NodeName       string    `json:"node_name" gorm:"index"`
	Source         string    `json:"source"`
	Service        string    `json:"service" gorm:"index"`
	ComposeService string    `json:"compose_service,omitempty" gorm:"index"`
	Event          string    `json:"event"`
	Content        string    `json:"content"`
	Metadata       string    `json:"metadata"`
	CorrelatedID   string    `json:"correlated_id,omitempty"`
	ReplaceID      string    `json:"replace_id,omitempty" gorm:"-"`
}
