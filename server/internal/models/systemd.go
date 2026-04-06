package models

// SystemdUnitConfig stores the list of systemd units to watch for a given node.
// One row per node; Units is a JSON-encoded []string.
type SystemdUnitConfig struct {
	NodeName string `gorm:"primaryKey" json:"node_name"`
	Units    string `json:"units"` // JSON []string
}
