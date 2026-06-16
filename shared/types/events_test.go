package types

import (
	"encoding/json"
	"testing"
)

func TestEntry_ImageRoundTripsThroughJSON(t *testing.T) {
	in := Entry{ID: "01ABC", Source: "docker", Service: "postgres", Event: "pull", Image: "postgres"}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Entry
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Image != "postgres" {
		t.Fatalf("expected image to survive round-trip, got %q", out.Image)
	}
}
