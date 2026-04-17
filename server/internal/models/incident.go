package models

import (
	"encoding/json"
	"time"
)

type Incident struct {
	ID          string     `gorm:"primaryKey" json:"-"`
	OpenedAt    time.Time  `gorm:"index" json:"-"`
	ResolvedAt  *time.Time `json:"-"`
	Status      string     `gorm:"index" json:"-"` // "open" | "resolved"
	Confidence  string     `gorm:"index" json:"-"` // "confirmed" | "suspected"
	Title       string     `json:"-"`
	Services    string     `json:"-"` // JSON []string stored as string
	RootCauseID string     `json:"-"`
	TriggerID   string     `json:"-"`
	NodeNames   string     `json:"-"` // JSON []string stored as string
	Metadata    string     `json:"-"` // JSON blob stored as string
}

// UnmarshalJSON accepts Services and NodeNames as JSON arrays and Metadata as a JSON
// object, converting them back to their internal string representation.
func (i *Incident) UnmarshalJSON(data []byte) error {
	var wire struct {
		ID          string          `json:"id"`
		OpenedAt    time.Time       `json:"opened_at"`
		ResolvedAt  *time.Time      `json:"resolved_at"`
		Status      string          `json:"status"`
		Confidence  string          `json:"confidence"`
		Title       string          `json:"title"`
		Services    []string        `json:"services"`
		RootCauseID string          `json:"root_cause_id"`
		TriggerID   string          `json:"trigger_id"`
		NodeNames   []string        `json:"node_names"`
		Metadata    json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	i.ID = wire.ID
	i.OpenedAt = wire.OpenedAt
	i.ResolvedAt = wire.ResolvedAt
	i.Status = wire.Status
	i.Confidence = wire.Confidence
	i.Title = wire.Title
	i.RootCauseID = wire.RootCauseID
	i.TriggerID = wire.TriggerID

	services, _ := json.Marshal(wire.Services)
	if s := string(services); s == "" || s == "null" {
		i.Services = "[]"
	} else {
		i.Services = s
	}
	nodeNames, _ := json.Marshal(wire.NodeNames)
	if s := string(nodeNames); s == "" || s == "null" {
		i.NodeNames = "[]"
	} else {
		i.NodeNames = s
	}
	if len(wire.Metadata) > 0 {
		i.Metadata = string(wire.Metadata)
	} else {
		i.Metadata = "{}"
	}
	return nil
}

// MarshalJSON outputs Incident with Services, NodeNames, and Metadata as proper JSON types.
func (i Incident) MarshalJSON() ([]byte, error) {
	var services []string
	if i.Services != "" {
		_ = json.Unmarshal([]byte(i.Services), &services)
	}
	if services == nil {
		services = []string{}
	}

	var nodeNames []string
	if i.NodeNames != "" {
		_ = json.Unmarshal([]byte(i.NodeNames), &nodeNames)
	}
	if nodeNames == nil {
		nodeNames = []string{}
	}

	metadata := json.RawMessage("{}")
	if i.Metadata != "" {
		metadata = json.RawMessage(i.Metadata)
	}

	type wire struct {
		ID          string          `json:"id"`
		OpenedAt    time.Time       `json:"opened_at"`
		ResolvedAt  *time.Time      `json:"resolved_at,omitempty"`
		Status      string          `json:"status"`
		Confidence  string          `json:"confidence"`
		Title       string          `json:"title"`
		Services    []string        `json:"services"`
		RootCauseID string          `json:"root_cause_id,omitempty"`
		TriggerID   string          `json:"trigger_id,omitempty"`
		NodeNames   []string        `json:"node_names"`
		Metadata    json.RawMessage `json:"metadata"`
	}
	return json.Marshal(wire{
		ID:          i.ID,
		OpenedAt:    i.OpenedAt,
		ResolvedAt:  i.ResolvedAt,
		Status:      i.Status,
		Confidence:  i.Confidence,
		Title:       i.Title,
		Services:    services,
		RootCauseID: i.RootCauseID,
		TriggerID:   i.TriggerID,
		NodeNames:   nodeNames,
		Metadata:    metadata,
	})
}

type IncidentEntry struct {
	IncidentID string `gorm:"primaryKey;index" json:"incident_id"`
	EntryID    string `gorm:"primaryKey;index" json:"entry_id"`
	Role       string `json:"role"`   // "trigger" | "cause" | "evidence" | "recovery" | "ai_cause"
	Score      int    `json:"score"`  // correlation score; 0 for non-cause roles
	Reason     string `json:"reason"` // empty for deterministic links; Ollama explanation for ai_cause
}
