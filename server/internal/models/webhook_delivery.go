package models

import "time"

type WebhookDelivery struct {
	ID                string    `gorm:"primaryKey" json:"id"`
	Source            string    `gorm:"not null;index" json:"source"`
	ReceivedAt        time.Time `gorm:"not null;index" json:"received_at"`
	PayloadSnippet    string    `gorm:"not null;default:''" json:"payload_snippet"`
	MatchedIncidentID *string   `gorm:"index" json:"matched_incident_id"`
	Status            string    `gorm:"not null;index" json:"status"`
	ErrorMessage      string    `gorm:"not null;default:''" json:"error_message"`
}
