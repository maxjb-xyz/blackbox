package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	dockerevents "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"
	"github.com/oklog/ulid/v2"

	"blackbox/shared/types"
)

var watchedActions = map[string]bool{
	"start":  true,
	"stop":   true,
	"die":    true,
	"create": true,
	"pull":   true,
	"delete": true,
}

func Watch(ctx context.Context, nodeName string, out chan<- types.Entry) {
	for {
		if err := watch(ctx, nodeName, out); err != nil {
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

func watch(ctx context.Context, nodeName string, out chan<- types.Entry) error {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	f := filters.NewArgs()
	f.Add("type", "container")
	f.Add("type", "image")

	msgCh, errCh := cli.Events(ctx, dockerevents.ListOptions{Filters: f})
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-errCh:
			if !ok {
				return fmt.Errorf("docker event error channel closed")
			}
			return err
		case msg, ok := <-msgCh:
			if !ok {
				return fmt.Errorf("docker event message channel closed")
			}
			action := string(msg.Action)
			if !watchedActions[action] {
				continue
			}
			entry := buildEntry(nodeName, msg)
			select {
			case out <- entry:
			default:
				log.Printf("docker watcher: dropped event node=%s action=%s id=%s type=%s", nodeName, action, msg.Actor.ID, string(msg.Type))
			}
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
		Timestamp: time.Now().UTC(),
		NodeName:  nodeName,
		Source:    "docker",
		Service:   service,
		Event:     action,
		Content:   content,
		Metadata:  string(metaBytes),
	}
}
