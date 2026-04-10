package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	dockerevents "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/oklog/ulid/v2"

	"blackbox/shared/types"
)

const debounceWindow = 3 * time.Second
const dockerLookupTimeout = 2 * time.Second
const logCaptureLines = 50
const logCaptureTimeout = 5 * time.Second

var watchedActions = map[string]bool{
	"start":  true,
	"stop":   true,
	"die":    true,
	"create": true,
	"pull":   true,
	"delete": true,
}

func Watch(ctx context.Context, nodeName string, out chan<- types.Entry) {
	collapser := newEventCollapser(nodeName, nil)

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
	defer func() {
		_ = cli.Close()
	}()

	f := filters.NewArgs()
	f.Add("type", "container")
	f.Add("type", "image")

	msgCh, errCh := cli.Events(ctx, dockerevents.ListOptions{Filters: f})
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	collapser.resolver = newServiceResolver(ctx, cli)

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

type dockerResolverClient interface {
	ContainerInspect(ctx context.Context, containerID string) (dockercontainer.InspectResponse, error)
	ContainerList(ctx context.Context, options dockercontainer.ListOptions) ([]dockercontainer.Summary, error)
	ContainerLogs(ctx context.Context, container string, options dockercontainer.LogsOptions) (io.ReadCloser, error)
}

type serviceResolver struct {
	ctx context.Context
	cli dockerResolverClient
}

type containerIdentity struct {
	service     string
	displayName string
}

func newServiceResolver(ctx context.Context, cli dockerResolverClient) *serviceResolver {
	return &serviceResolver{ctx: ctx, cli: cli}
}

func buildEntry(nodeName string, msg dockerevents.Message, resolver *serviceResolver) types.Entry {
	action := string(msg.Action)
	attrs := msg.Actor.Attributes
	name := attrs["name"]
	image := attrs["image"]
	exitCodeStr := attrs["exitCode"]

	resolvedService := ""
	displayName := ""
	if action == "pull" || action == "delete" || msg.Type == "image" {
		identity := resolver.resolveImageIdentity(msg.Actor.ID, attrs)
		resolvedService = identity.service
		displayName = firstNonEmpty(identity.displayName, resolvedService, cleanImageService(firstNonEmpty(attrs["name"], image, msg.Actor.ID)), msg.Actor.ID)
	} else {
		identity := resolver.resolveContainerIdentity(msg.Actor.ID, attrs, name)
		resolvedService = identity.service
		displayName = firstNonEmpty(identity.displayName, resolvedService, sanitizeContainerName(name), name, msg.Actor.ID)
	}

	var content string
	switch action {
	case "start":
		content = fmt.Sprintf("Container started: %s", displayName)
	case "stop":
		content = fmt.Sprintf("Container stopped: %s", displayName)
	case "create":
		content = fmt.Sprintf("Container created: %s", displayName)
		if image != "" {
			content = fmt.Sprintf("%s (image: %s)", content, image)
		}
	case "die":
		if exitCodeStr != "" && exitCodeStr != "0" {
			content = fmt.Sprintf("Container stopped: %s (exit code: %s)", displayName, exitCodeStr)
		} else {
			content = fmt.Sprintf("Container stopped cleanly: %s", displayName)
		}
	case "pull":
		content = fmt.Sprintf("Image pulled: %s", displayName)
	case "delete":
		content = fmt.Sprintf("Image deleted: %s", displayName)
	default:
		content = fmt.Sprintf("%s: %s", action, displayName)
	}

	metaBytes, _ := json.Marshal(attrs)

	return types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: messageTimestamp(msg),
		NodeName:  nodeName,
		Source:    "docker",
		Service:   resolvedService,
		Event:     action,
		Content:   content,
		Metadata:  string(metaBytes),
	}
}

type eventCollapser struct {
	nodeName string
	resolver *serviceResolver
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

func newEventCollapser(nodeName string, resolver *serviceResolver) *eventCollapser {
	return &eventCollapser{
		nodeName: nodeName,
		resolver: resolver,
		pending:  make(map[string]*pendingContainerEvent),
	}
}

func (c *eventCollapser) Handle(now time.Time, msg dockerevents.Message) []types.Entry {
	entries := c.FlushExpired(now)

	action := string(msg.Action)
	if msg.Type == "image" || action == "pull" || action == "delete" || action == "create" {
		return append(entries, buildEntry(c.nodeName, msg, c.resolver))
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
			return append(entries, buildCollapsedContainerEntry(c.nodeName, "start", []dockerevents.Message{msg}, c.resolver))
		}
		rawEvents := append(append([]dockerevents.Message{}, pending.rawEvents...), msg)
		delete(c.pending, containerID)
		return append(entries, buildCollapsedContainerEntry(c.nodeName, "restart", rawEvents, c.resolver))
	default:
		return append(entries, buildEntry(c.nodeName, msg, c.resolver))
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
		entries = append(entries, buildCollapsedContainerEntry(c.nodeName, "stop", pending.rawEvents, c.resolver))
		delete(c.pending, id)
	}

	return entries
}

func buildCollapsedContainerEntry(nodeName, event string, rawEvents []dockerevents.Message, resolver *serviceResolver) types.Entry {
	rawName := containerName(rawEvents)
	var attrs map[string]string
	if len(rawEvents) > 0 {
		attrs = rawEvents[len(rawEvents)-1].Actor.Attributes
	}

	containerID := ""
	if len(rawEvents) > 0 {
		containerID = rawEvents[len(rawEvents)-1].Actor.ID
	}
	identity := resolver.resolveContainerIdentity(containerID, attrs, rawName)
	service := identity.service
	displayName := firstNonEmpty(identity.displayName, service, sanitizeContainerName(rawName), rawName)
	if rawName == "" && service == "" && len(rawEvents) > 0 {
		service = rawEvents[len(rawEvents)-1].Actor.ID
		displayName = service
	}

	content := ""
	switch event {
	case "restart":
		content = fmt.Sprintf("Container restarted: %s", displayName)
	case "start":
		content = fmt.Sprintf("Container started: %s", displayName)
	case "stop":
		content = fmt.Sprintf("Container stopped: %s", displayName)
		if exitCode := exitCodeFromRawEvents(rawEvents); exitCode != "" {
			content = fmt.Sprintf("Container stopped: %s (exit code: %s)", displayName, exitCode)
		}
	default:
		content = fmt.Sprintf("%s: %s", event, displayName)
	}

	meta := map[string]interface{}{
		"raw_events": buildRawEvents(rawEvents),
	}
	if (event == "stop" || event == "restart") && resolver != nil && containerID != "" {
		if lines := resolver.captureContainerLogs(containerID); len(lines) > 0 {
			meta["log_snippet"] = lines
			meta["log_lines_captured"] = len(lines)
			meta["log_truncated"] = len(lines) == logCaptureLines
		}
	}
	metaBytes, _ := json.Marshal(meta)

	// Use the first stop event for collapsed stop timestamps so replacement
	// create/start events cannot appear to precede the stop that triggered them.
	timestamp := collapsedEntryTimestamp(event, rawEvents)

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

func collapsedEntryTimestamp(event string, rawEvents []dockerevents.Message) time.Time {
	// Callers currently build collapsed entries from at least one raw event, but
	// keep a defensive time.Now().UTC() fallback so future empty rawEvents slices
	// still produce a usable timestamp if messageTimestamp cannot be derived.
	if len(rawEvents) == 0 {
		return time.Now().UTC()
	}
	if event == "stop" {
		return messageTimestamp(rawEvents[0])
	}
	return messageTimestamp(rawEvents[len(rawEvents)-1])
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

func (r *serviceResolver) resolveContainerService(containerID string, attrs map[string]string, rawName string) string {
	return r.resolveContainerIdentity(containerID, attrs, rawName).service
}

func (r *serviceResolver) resolveImageService(imageID string, attrs map[string]string) string {
	return r.resolveImageIdentity(imageID, attrs).service
}

func (r *serviceResolver) resolveContainerIdentity(containerID string, attrs map[string]string, rawName string) containerIdentity {
	if identity := cleanContainerIdentity(attrs, ""); identity.service != "" {
		return identity
	}
	// Skip inspect when we already received real event attributes and have a name.
	// Docker events include all container labels, so if compose/swarm labels were
	// absent from the event attrs they won't appear on inspect either. Only fall
	// through to inspect when attrs is nil (no event labels at all).
	if rawName != "" && len(attrs) > 0 {
		return cleanContainerIdentity(nil, rawName)
	}
	if r != nil && r.cli != nil && containerID != "" {
		ctx, cancel := context.WithTimeout(r.parentContext(), dockerLookupTimeout)
		defer cancel()

		inspected, err := r.cli.ContainerInspect(ctx, containerID)
		if err == nil {
			inspectedName := strings.TrimPrefix(inspected.Name, "/")
			labels := map[string]string{}
			if inspected.Config != nil && inspected.Config.Labels != nil {
				labels = inspected.Config.Labels
			}
			if identity := cleanContainerIdentity(labels, inspectedName); identity.service != "" || identity.displayName != "" {
				return identity
			}
		}
	}
	return cleanContainerIdentity(nil, rawName)
}

func (r *serviceResolver) resolveImageIdentity(imageID string, attrs map[string]string) containerIdentity {
	ref := firstNonEmpty(attrs["name"], attrs["image"], imageID)
	if r != nil && r.cli != nil {
		if identity := r.findContainerIdentityForImage(imageID, ref); identity.service != "" || identity.displayName != "" {
			return identity
		}
	}
	imageService := cleanImageService(ref)
	return containerIdentity{
		service:     imageService,
		displayName: firstNonEmpty(imageService, ref, imageID),
	}
}

// captureContainerLogs fetches the last logCaptureLines lines from a container.
// Returns nil on any error — log capture is best-effort.
func (r *serviceResolver) captureContainerLogs(containerID string) []string {
	if r == nil || r.cli == nil || containerID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(r.parentContext(), logCaptureTimeout)
	defer cancel()

	tail := fmt.Sprintf("%d", logCaptureLines)
	rc, err := r.cli.ContainerLogs(ctx, containerID, dockercontainer.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	})
	if err != nil || rc == nil {
		return nil
	}
	defer func() { _ = rc.Close() }()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, rc); err != nil {
		return nil
	}

	lines := scanLogLines(&stdoutBuf)
	lines = append(lines, scanLogLines(&stderrBuf)...)
	return lines
}

func scanLogLines(r io.Reader) []string {
	var lines []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if trimmed := strings.TrimSpace(scanner.Text()); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func (r *serviceResolver) findContainerIdentityForImage(imageID, ref string) containerIdentity {
	ctx, cancel := context.WithTimeout(r.parentContext(), dockerLookupTimeout)
	defer cancel()

	containers, err := r.cli.ContainerList(ctx, dockercontainer.ListOptions{All: true})
	if err != nil {
		return containerIdentity{}
	}

	normalizedRef := normalizeImageRef(ref)
	shortRef := cleanImageService(ref)
	var repositoryMatch containerIdentity
	var shortNameMatch containerIdentity

	for _, summary := range containers {
		identity := cleanContainerIdentity(summary.Labels, summaryContainerName(summary.Names))
		if identity.service == "" {
			continue
		}
		if imageID != "" && summary.ImageID == imageID {
			return identity
		}
		if ref != "" && summary.Image == ref {
			return identity
		}
		if repositoryMatch.service == "" && normalizedRef != "" && normalizeImageRef(summary.Image) == normalizedRef {
			repositoryMatch = identity
		}
		if shortNameMatch.service == "" && shortRef != "" && cleanImageService(summary.Image) == shortRef {
			shortNameMatch = identity
		}
	}

	if repositoryMatch.service != "" {
		return repositoryMatch
	}
	return shortNameMatch
}

func cleanContainerService(attrs map[string]string, rawName string) string {
	if project := strings.TrimSpace(attrs["com.docker.compose.project"]); project != "" {
		return project
	}
	if service := normalizeSwarmServiceName(
		strings.TrimSpace(attrs["com.docker.swarm.service.name"]),
		strings.TrimSpace(attrs["com.docker.stack.namespace"]),
	); service != "" {
		return service
	}
	return sanitizeContainerName(rawName)
}

func cleanContainerIdentity(attrs map[string]string, rawName string) containerIdentity {
	service := cleanContainerService(attrs, rawName)
	return containerIdentity{
		service:     service,
		displayName: cleanContainerDisplayName(attrs, rawName, service),
	}
}

func cleanContainerDisplayName(attrs map[string]string, rawName, service string) string {
	project := strings.TrimSpace(attrs["com.docker.compose.project"])
	composeService := strings.TrimSpace(attrs["com.docker.compose.service"])
	switch {
	case project != "" && composeService != "":
		return fmt.Sprintf("%s · %s", project, composeService)
	case project != "":
		return project
	case service != "":
		return service
	default:
		return sanitizeContainerName(rawName)
	}
}

func normalizeSwarmServiceName(serviceName, stackName string) string {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return ""
	}
	if stackName != "" && strings.HasPrefix(serviceName, stackName+"_") {
		return strings.TrimPrefix(serviceName, stackName+"_")
	}
	return sanitizeContainerName(serviceName)
}

func sanitizeContainerName(name string) string {
	name = strings.TrimSpace(strings.TrimPrefix(name, "/"))
	if name == "" {
		return ""
	}

	parts := strings.Split(name, "_")
	// Handle swarm-style names like prefix_service_replica by extracting the service portion.
	if len(parts) >= 3 && isNumericToken(parts[len(parts)-1]) {
		service := strings.Join(parts[1:len(parts)-1], "_")
		if service != "" {
			return service
		}
	}
	// Strip generated hash-like prefixes such as hor2httb23tu3itbitb_service.
	if len(parts) >= 2 && shouldStripGeneratedPrefix(parts[0]) {
		return strings.Join(parts[1:], "_")
	}
	return name
}

func shouldStripGeneratedPrefix(prefix string) bool {
	if len(prefix) < 8 {
		return false
	}

	hasDigit := false
	for i := 0; i < len(prefix); i++ {
		ch := prefix[i]
		if ch >= '0' && ch <= '9' {
			hasDigit = true
			continue
		}
		if (ch < 'a' || ch > 'z') && ch != '-' {
			return false
		}
	}

	return hasDigit
}

func isNumericToken(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func summaryContainerName(names []string) string {
	for _, name := range names {
		trimmed := strings.TrimPrefix(strings.TrimSpace(name), "/")
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeImageRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "sha256:") {
		return ref
	}
	ref = stripImageTagAndDigest(ref)
	ref = strings.TrimPrefix(ref, "docker.io/")
	ref = strings.TrimPrefix(ref, "index.docker.io/")
	ref = strings.TrimPrefix(ref, "library/")
	return ref
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func cleanImageService(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "sha256:") {
		return ""
	}

	ref = stripImageTagAndDigest(ref)
	if idx := strings.LastIndex(ref, "/"); idx != -1 {
		ref = ref[idx+1:]
	}

	return ref
}

func (r *serviceResolver) parentContext() context.Context {
	if r == nil || r.ctx == nil {
		return context.Background()
	}
	return r.ctx
}

func stripImageTagAndDigest(ref string) string {
	if idx := strings.Index(ref, "@"); idx != -1 {
		ref = ref[:idx]
	}
	if idx := strings.LastIndex(ref, ":"); idx > strings.LastIndex(ref, "/") {
		ref = ref[:idx]
	}
	return ref
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
