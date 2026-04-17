package types

import (
	"encoding/json"
	"time"
)

type Entry struct {
	ID             string    `json:"id" gorm:"primaryKey;index:idx_entries_timestamp_id,priority:2"`
	Timestamp      time.Time `json:"timestamp" gorm:"index:idx_entries_timestamp_id,priority:1"`
	NodeName       string    `json:"node_name" gorm:"index"`
	Source         string    `json:"source"`
	Service        string    `json:"service" gorm:"index"`
	ComposeService string    `json:"compose_service,omitempty" gorm:"index"`
	Event          string    `json:"event"`
	Content        string    `json:"content"`
	Metadata       string    `json:"-" gorm:"column:metadata"`
	CorrelatedID   string    `json:"correlated_id,omitempty"`
	ReplaceID      string    `json:"replace_id,omitempty" gorm:"-"`
}

// entryJSON is the wire representation of Entry with Metadata as a JSON value.
type entryJSON struct {
	ID             string          `json:"id"`
	Timestamp      time.Time       `json:"timestamp"`
	NodeName       string          `json:"node_name"`
	Source         string          `json:"source"`
	Service        string          `json:"service"`
	ComposeService string          `json:"compose_service,omitempty"`
	Event          string          `json:"event"`
	Content        string          `json:"content"`
	Metadata       json.RawMessage `json:"metadata"`
	CorrelatedID   string          `json:"correlated_id,omitempty"`
	ReplaceID      string          `json:"replace_id,omitempty"`
}

// MarshalJSON outputs Metadata as a proper JSON value instead of a string.
func (e Entry) MarshalJSON() ([]byte, error) {
	meta := json.RawMessage("{}")
	if e.Metadata != "" {
		meta = json.RawMessage(e.Metadata)
	}
	return json.Marshal(entryJSON{
		ID:             e.ID,
		Timestamp:      e.Timestamp,
		NodeName:       e.NodeName,
		Source:         e.Source,
		Service:        e.Service,
		ComposeService: e.ComposeService,
		Event:          e.Event,
		Content:        e.Content,
		Metadata:       meta,
		CorrelatedID:   e.CorrelatedID,
		ReplaceID:      e.ReplaceID,
	})
}

// UnmarshalJSON accepts Metadata as either a JSON object (new) or a JSON string (legacy).
func (e *Entry) UnmarshalJSON(data []byte) error {
	var v entryJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	e.ID = v.ID
	e.Timestamp = v.Timestamp
	e.NodeName = v.NodeName
	e.Source = v.Source
	e.Service = v.Service
	e.ComposeService = v.ComposeService
	e.Event = v.Event
	e.Content = v.Content
	e.CorrelatedID = v.CorrelatedID
	e.ReplaceID = v.ReplaceID

	if len(v.Metadata) > 0 {
		// Legacy agents send metadata as a JSON string containing JSON.
		// New agents send it directly as a JSON object.
		var s string
		if json.Unmarshal(v.Metadata, &s) == nil {
			// Unwrapped a legacy string. Validate the inner value is itself
			// valid JSON so MarshalJSON can safely emit it as json.RawMessage.
			var tmp json.RawMessage
			if json.Unmarshal([]byte(s), &tmp) == nil {
				e.Metadata = s
			} else {
				// The inner string is plain text, not JSON; wrap it to keep
				// the value without producing invalid RawMessage output.
				wrapped, _ := json.Marshal(map[string]string{"value": s})
				e.Metadata = string(wrapped)
			}
		} else {
			e.Metadata = string(v.Metadata)
		}
	}
	return nil
}
