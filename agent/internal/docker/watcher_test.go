package docker

import (
	"encoding/json"
	"testing"
	"time"

	dockerevents "github.com/docker/docker/api/types/events"
)

func TestEventCollapser_CollapsesRestartSequence(t *testing.T) {
	collapser := newEventCollapser("node-1")
	base := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)

	if entries := collapser.Handle(base, testDockerMessage(base, "container", "stop", "abc123", "traefik", "")); len(entries) != 0 {
		t.Fatalf("expected stop to be buffered, got %d entries", len(entries))
	}
	if entries := collapser.Handle(base.Add(time.Second), testDockerMessage(base.Add(time.Second), "container", "die", "abc123", "traefik", "137")); len(entries) != 0 {
		t.Fatalf("expected die to be buffered, got %d entries", len(entries))
	}

	entries := collapser.Handle(base.Add(2*time.Second), testDockerMessage(base.Add(2*time.Second), "container", "start", "abc123", "traefik", ""))
	if len(entries) != 1 {
		t.Fatalf("expected 1 restart entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Event != "restart" {
		t.Fatalf("expected restart event, got %q", entry.Event)
	}
	if entry.Content != "Container restarted: traefik" {
		t.Fatalf("unexpected content: %q", entry.Content)
	}
	if entry.Service != "traefik" {
		t.Fatalf("expected service traefik, got %q", entry.Service)
	}
	if !entry.Timestamp.Equal(base.Add(2 * time.Second)) {
		t.Fatalf("expected timestamp %v, got %v", base.Add(2*time.Second), entry.Timestamp)
	}

	var meta struct {
		RawEvents []map[string]interface{} `json:"raw_events"`
	}
	if err := json.Unmarshal([]byte(entry.Metadata), &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if len(meta.RawEvents) != 3 {
		t.Fatalf("expected 3 raw events, got %d", len(meta.RawEvents))
	}
	if meta.RawEvents[0]["action"] != "stop" || meta.RawEvents[1]["action"] != "die" || meta.RawEvents[2]["action"] != "start" {
		t.Fatalf("unexpected raw event actions: %#v", meta.RawEvents)
	}
}

func TestEventCollapser_EmitsStopAfterDebounce(t *testing.T) {
	collapser := newEventCollapser("node-1")
	base := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)

	if entries := collapser.Handle(base, testDockerMessage(base, "container", "stop", "abc123", "traefik", "")); len(entries) != 0 {
		t.Fatalf("expected stop to be buffered, got %d entries", len(entries))
	}
	if entries := collapser.Handle(base.Add(time.Second), testDockerMessage(base.Add(time.Second), "container", "die", "abc123", "traefik", "137")); len(entries) != 0 {
		t.Fatalf("expected die to be buffered, got %d entries", len(entries))
	}

	entries := collapser.FlushExpired(base.Add(4*time.Second + time.Millisecond))
	if len(entries) != 1 {
		t.Fatalf("expected 1 stop entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Event != "stop" {
		t.Fatalf("expected stop event, got %q", entry.Event)
	}
	if entry.Content != "Container stopped: traefik (exit code: 137)" {
		t.Fatalf("unexpected content: %q", entry.Content)
	}
	if !entry.Timestamp.Equal(base.Add(time.Second)) {
		t.Fatalf("expected timestamp %v, got %v", base.Add(time.Second), entry.Timestamp)
	}

	var meta struct {
		RawEvents []map[string]interface{} `json:"raw_events"`
	}
	if err := json.Unmarshal([]byte(entry.Metadata), &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if len(meta.RawEvents) != 2 {
		t.Fatalf("expected 2 raw events, got %d", len(meta.RawEvents))
	}
}

func TestEventCollapser_EmitsImmediateStartAndPassesThroughImageEvents(t *testing.T) {
	collapser := newEventCollapser("node-1")
	base := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)

	startEntries := collapser.Handle(base, testDockerMessage(base, "container", "start", "abc123", "traefik", ""))
	if len(startEntries) != 1 {
		t.Fatalf("expected 1 start entry, got %d", len(startEntries))
	}
	if startEntries[0].Event != "start" || startEntries[0].Content != "Container started: traefik" {
		t.Fatalf("unexpected start entry: %+v", startEntries[0])
	}

	var startMeta struct {
		RawEvents []map[string]interface{} `json:"raw_events"`
	}
	if err := json.Unmarshal([]byte(startEntries[0].Metadata), &startMeta); err != nil {
		t.Fatalf("unmarshal start metadata: %v", err)
	}
	if len(startMeta.RawEvents) != 1 {
		t.Fatalf("expected 1 raw event for start, got %d", len(startMeta.RawEvents))
	}

	imageEntries := collapser.Handle(base.Add(time.Second), testDockerMessage(base.Add(time.Second), "image", "pull", "sha256:abc123", "", ""))
	if len(imageEntries) != 1 {
		t.Fatalf("expected 1 image entry, got %d", len(imageEntries))
	}
	if imageEntries[0].Event != "pull" || imageEntries[0].Content != "image pulled: sha256:abc123" {
		t.Fatalf("unexpected image entry: %+v", imageEntries[0])
	}
}

func testDockerMessage(ts time.Time, typ, action, id, name, exitCode string) dockerevents.Message {
	attrs := map[string]string{}
	if name != "" {
		attrs["name"] = name
	}
	if exitCode != "" {
		attrs["exitCode"] = exitCode
	}

	return dockerevents.Message{
		Type:     dockerevents.Type(typ),
		Action:   dockerevents.Action(action),
		Time:     ts.Unix(),
		TimeNano: ts.UnixNano(),
		Actor: dockerevents.Actor{
			ID:         id,
			Attributes: attrs,
		},
	}
}
