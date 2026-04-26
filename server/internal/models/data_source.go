package models

import (
	"slices"
	"time"
)

const (
	ScopeAgent  = "agent"
	ScopeServer = "server"
)

// docker is a virtual built-in source with no DataSourceInstance rows; keep it in
// agentScopedSingletonSourceTypes for compatibility even though knownTypes filters creation.
var agentScopedSingletonSourceTypes = []string{"docker", "systemd", "filewatcher"}
var serverScopedSingletonSourceTypes = []string{"webhook_uptime_kuma", "webhook_watchtower"}

func GetAgentScopedSingletonSourceTypes() []string {
	return slices.Clone(agentScopedSingletonSourceTypes)
}

func GetServerScopedSingletonSourceTypes() []string {
	return slices.Clone(serverScopedSingletonSourceTypes)
}

// DataSourceInstance is a configured instance of a source type.
// Config is a JSON blob whose shape is defined per Type.
type DataSourceInstance struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Type      string    `gorm:"not null;index" json:"type"`
	Scope     string    `gorm:"not null" json:"scope"`          // ScopeServer | ScopeAgent
	NodeID    *string   `gorm:"index" json:"node_id,omitempty"` // Stores the node name for agent-scoped sources.
	Name      string    `gorm:"not null" json:"name"`
	Config    string    `gorm:"not null;default:'{}'" json:"config"` // JSON blob
	Enabled   bool      `gorm:"not null" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func IsSingletonSourceType(scope, typ string) bool {
	switch scope {
	case ScopeAgent:
		return slices.Contains(agentScopedSingletonSourceTypes, typ)
	case ScopeServer:
		return slices.Contains(serverScopedSingletonSourceTypes, typ)
	default:
		return false
	}
}
