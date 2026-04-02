package types

import "time"

type Entry struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	Timestamp    time.Time `json:"timestamp" gorm:"index"`
	NodeName     string    `json:"node_name" gorm:"index"`
	Source       string    `json:"source"`
	Service      string    `json:"service" gorm:"index"`
	Event        string    `json:"event"`
	Content      string    `json:"content"`
	Metadata     string    `json:"metadata"`
	CorrelatedID string    `json:"correlated_id,omitempty"`
}
