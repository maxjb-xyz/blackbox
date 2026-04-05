package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	dockerevents "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/pkg/stdcopy"

	"blackbox/shared/types"
)

func TestEventCollapser_CollapsesRestartSequence(t *testing.T) {
	collapser := newEventCollapser("node-1", nil)
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
	collapser := newEventCollapser("node-1", nil)
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
	collapser := newEventCollapser("node-1", nil)
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

	imageEntries := collapser.Handle(
		base.Add(time.Second),
		testDockerMessage(base.Add(time.Second), "image", "pull", "sha256:abc123", "lscr.io/linuxserver/sonarr:latest", ""),
	)
	if len(imageEntries) != 1 {
		t.Fatalf("expected 1 image entry, got %d", len(imageEntries))
	}
	if imageEntries[0].Event != "pull" || imageEntries[0].Content != "Image pulled: sonarr" {
		t.Fatalf("unexpected image entry: %+v", imageEntries[0])
	}
	if imageEntries[0].Service != "sonarr" {
		t.Fatalf("expected image service sonarr, got %q", imageEntries[0].Service)
	}
}

func TestBuildEntry_StripsGeneratedPrefixesFromContainerNames(t *testing.T) {
	entry := buildEntry(
		"node-1",
		testDockerMessage(time.Now().UTC(), "container", "create", "abc123", "hor2httb23tu3itbitb_sonarr", ""),
		nil,
	)

	if entry.Service != "sonarr" {
		t.Fatalf("expected service sonarr, got %q", entry.Service)
	}
	if entry.Content != "Container created: sonarr" {
		t.Fatalf("unexpected content: %q", entry.Content)
	}
}

func TestBuildEntry_UsesContainerLookupForImagePullService(t *testing.T) {
	resolver := newServiceResolver(context.Background(), fakeDockerResolverClient{
		containers: []dockercontainer.Summary{
			{
				Image:   "lscr.io/linuxserver/sonarr:latest",
				ImageID: "sha256:abc123",
				Names:   []string{"/generatedprefix_sonarr"},
				Labels:  map[string]string{"com.docker.compose.service": "sonarr"},
			},
		},
	})

	entry := buildEntry(
		"node-1",
		testDockerMessage(time.Now().UTC(), "image", "pull", "sha256:abc123", "ghcr.io/example/not-the-service-name:latest", ""),
		resolver,
	)

	if entry.Service != "sonarr" {
		t.Fatalf("expected service sonarr, got %q", entry.Service)
	}
	if entry.Content != "Image pulled: sonarr" {
		t.Fatalf("unexpected content: %q", entry.Content)
	}
}

func TestResolveContainerService_FallsBackWhenInspectFails(t *testing.T) {
	resolver := newServiceResolver(context.Background(), fakeDockerResolverClient{
		inspectErr: assertiveResolverError("inspect failed"),
	})

	service := resolver.resolveContainerService("abc123", nil, "hor2httb23tu3itbitb_sonarr")
	if service != "sonarr" {
		t.Fatalf("expected sanitized fallback service sonarr, got %q", service)
	}
}

func TestResolveImageService_FallsBackWhenContainerListFails(t *testing.T) {
	resolver := newServiceResolver(context.Background(), fakeDockerResolverClient{
		containerErr: assertiveResolverError("container list failed"),
	})

	service := resolver.resolveImageService("sha256:abc123", map[string]string{
		"name": "lscr.io/linuxserver/sonarr:latest",
	})
	if service != "sonarr" {
		t.Fatalf("expected short image fallback service sonarr, got %q", service)
	}
}

func TestCaptureContainerLogs_DemuxesDockerFrames(t *testing.T) {
	var mux bytes.Buffer
	_, err := stdcopy.NewStdWriter(&mux, stdcopy.Stdout).Write([]byte("stdout line\n"))
	if err != nil {
		t.Fatalf("write stdout frame: %v", err)
	}
	_, err = stdcopy.NewStdWriter(&mux, stdcopy.Stderr).Write([]byte("stderr line\n"))
	if err != nil {
		t.Fatalf("write stderr frame: %v", err)
	}

	resolver := newServiceResolver(context.Background(), fakeDockerResolverClient{
		logData: mux.Bytes(),
	})

	lines := resolver.captureContainerLogs("abc123")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d (%v)", len(lines), lines)
	}
	if lines[0] != "stdout line" || lines[1] != "stderr line" {
		t.Fatalf("unexpected log lines: %v", lines)
	}
}

func TestBuildCollapsedContainerEntry_UsesSwarmServiceLabel(t *testing.T) {
	base := time.Now().UTC()
	entry := buildCollapsedContainerEntry("node-1", "restart", []dockerevents.Message{
		testDockerMessageWithAttrs(base, "container", "stop", "abc123", "stack_sonarr.1.xxxxx", "", map[string]string{
			"com.docker.swarm.service.name": "stack_sonarr",
			"com.docker.stack.namespace":    "stack",
		}),
		testDockerMessageWithAttrs(base.Add(time.Second), "container", "start", "abc123", "stack_sonarr.1.xxxxx", "", map[string]string{
			"com.docker.swarm.service.name": "stack_sonarr",
			"com.docker.stack.namespace":    "stack",
		}),
	}, nil)

	if entry.Service != "sonarr" {
		t.Fatalf("expected service sonarr, got %q", entry.Service)
	}
	if entry.Content != "Container restarted: sonarr" {
		t.Fatalf("unexpected content: %q", entry.Content)
	}
}

func TestRunWatchLoop_PreservesBufferedEventsAcrossReconnect(t *testing.T) {
	base := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collapser := newEventCollapser("node-1", nil)
	out := make(chan types.Entry, 4)
	tickCh := make(chan time.Time)

	msgCh1 := make(chan dockerevents.Message, 1)
	errCh1 := make(chan error)
	msgCh1 <- testDockerMessage(base, "container", "stop", "abc123", "traefik", "")
	close(msgCh1)

	done1 := make(chan error, 1)
	go func() {
		done1 <- runWatchLoop(ctx, "node-1", out, msgCh1, errCh1, tickCh, collapser, func() time.Time {
			return base
		})
	}()

	if err := <-done1; err == nil || err.Error() != "docker event message channel closed" {
		t.Fatalf("expected closed message channel error, got %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no emitted entries before reconnect, got %d", len(out))
	}

	msgCh2 := make(chan dockerevents.Message, 1)
	errCh2 := make(chan error)
	msgCh2 <- testDockerMessage(base.Add(2*time.Second), "container", "start", "abc123", "traefik", "")
	close(msgCh2)

	done2 := make(chan error, 1)
	go func() {
		done2 <- runWatchLoop(ctx, "node-1", out, msgCh2, errCh2, tickCh, collapser, func() time.Time {
			return base.Add(2 * time.Second)
		})
	}()

	entry := <-out
	if entry.Event != "restart" {
		t.Fatalf("expected restart event after reconnect, got %q", entry.Event)
	}
	if entry.Service != "traefik" {
		t.Fatalf("expected service traefik, got %q", entry.Service)
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
	if meta.RawEvents[0]["action"] != "stop" || meta.RawEvents[1]["action"] != "start" {
		t.Fatalf("unexpected raw event actions: %#v", meta.RawEvents)
	}

	if err := <-done2; err == nil || err.Error() != "docker event message channel closed" {
		t.Fatalf("expected closed message channel error after reconnect, got %v", err)
	}
}

func TestRunWatchLoop_EmitsExpiredStopBeforeLateStartWithoutTicker(t *testing.T) {
	base := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collapser := newEventCollapser("node-1", nil)
	out := make(chan types.Entry, 4)
	tickCh := make(chan time.Time)
	msgCh := make(chan dockerevents.Message, 2)
	errCh := make(chan error)

	msgCh <- testDockerMessage(base, "container", "stop", "abc123", "traefik", "")
	msgCh <- testDockerMessage(base.Add(4*time.Second), "container", "start", "abc123", "traefik", "")
	close(msgCh)

	times := []time.Time{base, base.Add(4 * time.Second), base.Add(4 * time.Second)}
	index := 0
	now := func() time.Time {
		current := times[index]
		if index < len(times)-1 {
			index++
		}
		return current
	}

	done := make(chan error, 1)
	go func() {
		done <- runWatchLoop(ctx, "node-1", out, msgCh, errCh, tickCh, collapser, now)
	}()

	first := <-out
	second := <-out

	if first.Event != "stop" {
		t.Fatalf("expected expired stop entry first, got %q", first.Event)
	}
	if second.Event != "start" {
		t.Fatalf("expected start entry second, got %q", second.Event)
	}

	if err := <-done; err == nil || err.Error() != "docker event message channel closed" {
		t.Fatalf("expected closed message channel error, got %v", err)
	}
}

func testDockerMessage(ts time.Time, typ, action, id, name, exitCode string) dockerevents.Message {
	return testDockerMessageWithAttrs(ts, typ, action, id, name, exitCode, nil)
}

func testDockerMessageWithAttrs(ts time.Time, typ, action, id, name, exitCode string, extra map[string]string) dockerevents.Message {
	attrs := map[string]string{}
	if name != "" {
		attrs["name"] = name
	}
	if exitCode != "" {
		attrs["exitCode"] = exitCode
	}
	for key, value := range extra {
		attrs[key] = value
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

type fakeDockerResolverClient struct {
	inspectResponse dockercontainer.InspectResponse
	inspectErr      error
	containers      []dockercontainer.Summary
	containerErr    error
	logData         []byte
	logErr          error
}

func (f fakeDockerResolverClient) ContainerInspect(_ context.Context, _ string) (dockercontainer.InspectResponse, error) {
	return f.inspectResponse, f.inspectErr
}

func (f fakeDockerResolverClient) ContainerList(_ context.Context, _ dockercontainer.ListOptions) ([]dockercontainer.Summary, error) {
	return f.containers, f.containerErr
}

func (f fakeDockerResolverClient) ContainerLogs(_ context.Context, _ string, opts dockercontainer.LogsOptions) (io.ReadCloser, error) {
	if f.logErr != nil {
		return nil, f.logErr
	}
	expectedTail := fmt.Sprintf("%d", logCaptureLines)
	if !opts.ShowStdout || !opts.ShowStderr || opts.Tail != expectedTail {
		return nil, assertiveResolverError("unexpected container log options")
	}
	if f.logData == nil {
		return nil, nil
	}
	return io.NopCloser(bytes.NewReader(f.logData)), nil
}

type assertiveResolverError string

func (e assertiveResolverError) Error() string {
	return string(e)
}
