package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	dockerevents "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"
	"github.com/oklog/ulid/v2"

	"blackbox/shared/types"
)

const debounceWindow = 3 * time.Second

var watchedActions = map[string]bool{
	"start":  true,
	"stop":   true,
	"die":    true,
	"create": true,
	"pull":   true,
	"delete": true,
}

func Watch(ctx context.Context, nodeName string, out chan<- types.Entry) {
	collapser := newEventCollapser(nodeName)

	for {
		if err := watch(ctx, nodeName, out, collapser); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("docker watcher: error: %v — reconnecting in 5s", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		} else {
			return
		}
	}
}

func watch(ctx context.Context, nodeName string, out chan<- types.Entry, collapser *eventCollapser) error {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	f := filters.NewArgs()
	f.Add("type", "container")
	f.Add("type", "image")

	msgCh, errCh := cli.Events(ctx, dockerevents.ListOptions{Filters: f})
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	return runWatchLoop(ctx, nodeName, out, msgCh, errCh, ticker.C, collapser, func() time.Time {
		return time.Now().UTC()
	})
}

func runWatchLoop(
	ctx context.Context,
	nodeName string,
	out chan<- types.Entry,
	msgCh <-chan dockerevents.Message,
	errCh <-chan error,
	tickCh <-chan time.Time,
	collapser *eventCollapser,
	now func() time.Time,
) error {
	for {
		select {
		case <-ctx.Done():
			emitEntries(nodeName, out, collapser.FlushExpired(now()))
			return nil
		case <-tickCh:
			emitEntries(nodeName, out, collapser.FlushExpired(now()))
		case err, ok := <-errCh:
			emitEntries(nodeName, out, collapser.FlushExpired(now()))
			if !ok {
				return fmt.Errorf("docker event error channel closed")
			}
			return err
		case msg, ok := <-msgCh:
			current := now()
			if !ok {
				emitEntries(nodeName, out, collapser.FlushExpired(current))
				return fmt.Errorf("docker event message channel closed")
			}
			action := string(msg.Action)
			if !watchedActions[action] {
				continue
			}
			emitEntries(nodeName, out, collapser.Handle(current, msg))
		}
	}
}

func buildEntry(nodeName string, msg dockerevents.Message) types.Entry {
	action := string(msg.Action)
	attrs := msg.Actor.Attributes
	name := attrs["name"]
	image := attrs["image"]
	exitCodeStr := attrs["exitCode"]

	var content string
	switch action {
	case "start":
		content = fmt.Sprintf("container %s started", name)
	case "stop":
		content = fmt.Sprintf("container %s stopped", name)
	case "create":
		content = fmt.Sprintf("container %s created (image: %s)", name, image)
	case "die":
		if exitCodeStr != "" && exitCodeStr != "0" {
			content = fmt.Sprintf("container %s died (exit code: %s)", name, exitCodeStr)
		} else {
			content = fmt.Sprintf("container %s stopped cleanly", name)
		}
	case "pull":
		content = fmt.Sprintf("image pulled: %s", msg.Actor.ID)
	case "delete":
		content = fmt.Sprintf("image deleted: %s", msg.Actor.ID)
	default:
		content = fmt.Sprintf("%s: %s", action, name)
	}

	metaBytes, _ := json.Marshal(attrs)
	service := name
	if action == "pull" || action == "delete" {
		service = msg.Actor.ID
	}

	return types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: messageTimestamp(msg),
		NodeName:  nodeName,
		Source:    "docker",
		Service:   service,
		Event:     action,
		Content:   content,
		Metadata:  string(metaBytes),
	}
}

type eventCollapser struct {
	nodeName string
	pending  map[string]*pendingContainerEvent
}

type pendingContainerEvent struct {
	rawEvents []dockerevents.Message
	deadline  time.Time
}

type rawDockerEvent struct {
	Action     string            `json:"action"`
	Type       string            `json:"type"`
	ID         string            `json:"id,omitempty"`
	Time       string            `json:"time"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

func newEventCollapser(nodeName string) *eventCollapser {
	return &eventCollapser{
		nodeName: nodeName,
		pending:  make(map[string]*pendingContainerEvent),
	}
}

func (c *eventCollapser) Handle(now time.Time, msg dockerevents.Message) []types.Entry {
	entries := c.FlushExpired(now)

	action := string(msg.Action)
	if msg.Type == "image" || action == "pull" || action == "delete" || action == "create" {
		return append(entries, buildEntry(c.nodeName, msg))
	}

	containerID := msg.Actor.ID
	switch action {
	case "stop", "die":
		pending := c.pending[containerID]
		if pending == nil {
			pending = &pendingContainerEvent{}
			c.pending[containerID] = pending
		}
		pending.rawEvents = append(pending.rawEvents, msg)
		pending.deadline = now.Add(debounceWindow)
		return entries
	case "start":
		pending := c.pending[containerID]
		if pending == nil {
			return append(entries, buildCollapsedContainerEntry(c.nodeName, "start", []dockerevents.Message{msg}))
		}
		rawEvents := append(append([]dockerevents.Message{}, pending.rawEvents...), msg)
		delete(c.pending, containerID)
		return append(entries, buildCollapsedContainerEntry(c.nodeName, "restart", rawEvents))
	default:
		return append(entries, buildEntry(c.nodeName, msg))
	}
}

func (c *eventCollapser) FlushExpired(now time.Time) []types.Entry {
	var ids []string
	for id, pending := range c.pending {
		if !pending.deadline.After(now) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	entries := make([]types.Entry, 0, len(ids))
	for _, id := range ids {
		pending := c.pending[id]
		entries = append(entries, buildCollapsedContainerEntry(c.nodeName, "stop", pending.rawEvents))
		delete(c.pending, id)
	}

	return entries
}

func buildCollapsedContainerEntry(nodeName, event string, rawEvents []dockerevents.Message) types.Entry {
	service := containerName(rawEvents)
	if service == "" && len(rawEvents) > 0 {
		service = rawEvents[len(rawEvents)-1].Actor.ID
	}

	content := ""
	switch event {
	case "restart":
		content = fmt.Sprintf("Container restarted: %s", service)
	case "start":
		content = fmt.Sprintf("Container started: %s", service)
	case "stop":
		content = fmt.Sprintf("Container stopped: %s", service)
		if exitCode := exitCodeFromRawEvents(rawEvents); exitCode != "" {
			content = fmt.Sprintf("Container stopped: %s (exit code: %s)", service, exitCode)
		}
	default:
		content = fmt.Sprintf("%s: %s", event, service)
	}

	metaBytes, _ := json.Marshal(map[string]interface{}{
		"raw_events": buildRawEvents(rawEvents),
	})

	// Callers currently build collapsed entries from at least one raw event, but
	// keep a defensive time.Now().UTC() fallback so future empty rawEvents slices
	// still produce a usable timestamp if messageTimestamp cannot be derived.
	timestamp := time.Now().UTC()
	if len(rawEvents) > 0 {
		timestamp = messageTimestamp(rawEvents[len(rawEvents)-1])
	}

	return types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: timestamp,
		NodeName:  nodeName,
		Source:    "docker",
		Service:   service,
		Event:     event,
		Content:   content,
		Metadata:  string(metaBytes),
	}
}

func buildRawEvents(rawEvents []dockerevents.Message) []rawDockerEvent {
	events := make([]rawDockerEvent, 0, len(rawEvents))
	for _, raw := range rawEvents {
		attrs := raw.Actor.Attributes
		if attrs == nil {
			attrs = map[string]string{}
		}
		events = append(events, rawDockerEvent{
			Action:     string(raw.Action),
			Type:       string(raw.Type),
			ID:         raw.Actor.ID,
			Time:       messageTimestamp(raw).Format(time.RFC3339Nano),
			Attributes: attrs,
		})
	}
	return events
}

func containerName(rawEvents []dockerevents.Message) string {
	for i := len(rawEvents) - 1; i >= 0; i-- {
		if name := rawEvents[i].Actor.Attributes["name"]; name != "" {
			return name
		}
	}
	return ""
}

func exitCodeFromRawEvents(rawEvents []dockerevents.Message) string {
	for i := len(rawEvents) - 1; i >= 0; i-- {
		if exitCode := rawEvents[i].Actor.Attributes["exitCode"]; exitCode != "" {
			return exitCode
		}
	}
	return ""
}

func messageTimestamp(msg dockerevents.Message) time.Time {
	if msg.TimeNano != 0 {
		return time.Unix(0, msg.TimeNano).UTC()
	}
	if msg.Time != 0 {
		return time.Unix(msg.Time, 0).UTC()
	}
	return time.Now().UTC()
}

func emitEntries(nodeName string, out chan<- types.Entry, entries []types.Entry) {
	for _, entry := range entries {
		select {
		case out <- entry:
		default:
			log.Printf(
				"docker watcher: dropped event node=%s action=%s service=%s source=%s",
				nodeName,
				entry.Event,
				entry.Service,
				entry.Source,
			)
		}
	}
}
