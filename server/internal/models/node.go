package models

import "time"

type Node struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"uniqueIndex" json:"name"`
	LastSeen     time.Time `gorm:"index" json:"last_seen"`
	AgentVersion string    `json:"agent_version"`
	IPAddress    string    `json:"ip_address"`
	OsInfo       string    `json:"os_info"`
}
