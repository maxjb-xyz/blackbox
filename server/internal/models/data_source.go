package models

import "time"

// DataSourceInstance is a configured instance of a source type.
// Config is a JSON blob whose shape is defined per Type.
type DataSourceInstance struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Type      string    `gorm:"not null;index" json:"type"`
	Scope     string    `gorm:"not null" json:"scope"`          // "server" | "agent"
	NodeID    *string   `gorm:"index" json:"node_id,omitempty"` // Stores the node name for agent-scoped sources.
	Name      string    `gorm:"not null" json:"name"`
	Config    string    `gorm:"not null;default:'{}'" json:"config"` // JSON blob
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
