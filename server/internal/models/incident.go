package models

import "time"

type Incident struct {
	ID          string     `gorm:"primaryKey" json:"id"`
	OpenedAt    time.Time  `gorm:"index" json:"opened_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	Status      string     `gorm:"index" json:"status"`     // "open" | "resolved"
	Confidence  string     `gorm:"index" json:"confidence"` // "confirmed" | "suspected"
	Title       string     `json:"title"`
	Services    string     `json:"services"` // JSON []string
	RootCauseID string     `json:"root_cause_id,omitempty"`
	TriggerID   string     `json:"trigger_id,omitempty"`
	NodeNames   string     `json:"node_names"` // JSON []string
	Metadata    string     `json:"metadata"`   // JSON blob: ai_analysis, etc.
}

type IncidentEntry struct {
	IncidentID string `gorm:"primaryKey;index" json:"incident_id"`
	EntryID    string `gorm:"primaryKey;index" json:"entry_id"`
	Role       string `json:"role"`  // "trigger" | "cause" | "evidence" | "recovery"
	Score      int    `json:"score"` // correlation score; 0 for non-cause roles
}
