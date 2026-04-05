package files

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"blackbox/shared/types"
	"github.com/fsnotify/fsnotify"
)

func TestWatch_EmitsWriteForConfigFile(t *testing.T) {
	root := t.TempDir()
	serviceDir := filepath.Join(root, "sonarr")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("mkdir service dir: %v", err)
	}

	target := filepath.Join(serviceDir, "docker-compose.yml")
	if err := os.WriteFile(target, []byte("services:\n  sonarr:\n    image: lscr.io/linuxserver/sonarr\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan types.Entry, 4)
	if count := Watch(ctx, "node-1", []string{root}, nil, out); count == 0 {
		t.Fatal("expected watcher to register at least one directory")
	}

	if err := os.WriteFile(target, []byte("services:\n  sonarr:\n    image: lscr.io/linuxserver/sonarr:latest\n"), 0o644); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}

	entry := waitForEntry(t, out)
	if entry.Source != "files" {
		t.Fatalf("entry source = %q, want files", entry.Source)
	}
	if entry.Event != "write" {
		t.Fatalf("entry event = %q, want write", entry.Event)
	}
	if entry.Service != filepath.Clean(root) {
		t.Fatalf("entry service = %q, want %q", entry.Service, filepath.Clean(root))
	}

	var meta map[string]string
	if err := json.Unmarshal([]byte(entry.Metadata), &meta); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if got := meta["path"]; got != target {
		t.Fatalf("metadata path = %q, want %q", got, target)
	}
}

func TestWatch_FollowsSymlinkedRootDirectory(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real-stacks")
	if err := os.MkdirAll(filepath.Join(realRoot, "sonarr"), 0o755); err != nil {
		t.Fatalf("mkdir real root: %v", err)
	}

	target := filepath.Join(realRoot, "sonarr", "docker-compose.yml")
	if err := os.WriteFile(target, []byte("services:\n  sonarr:\n    image: lscr.io/linuxserver/sonarr\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	linkRoot := filepath.Join(base, "watch-stacks")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatalf("symlink root: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan types.Entry, 4)
	if count := Watch(ctx, "node-1", []string{linkRoot}, nil, out); count == 0 {
		t.Fatal("expected watcher to register a symlinked root")
	}

	if err := os.WriteFile(target, []byte("services:\n  sonarr:\n    image: lscr.io/linuxserver/sonarr:testing\n"), 0o644); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}

	entry := waitForEntry(t, out)
	wantPath := filepath.Join(linkRoot, "sonarr", "docker-compose.yml")
	if entry.Service != filepath.Clean(linkRoot) {
		t.Fatalf("entry service = %q, want %q", entry.Service, filepath.Clean(linkRoot))
	}
	if entry.Content != "file write: "+wantPath {
		t.Fatalf("entry content = %q, want %q", entry.Content, "file write: "+wantPath)
	}

	var meta map[string]string
	if err := json.Unmarshal([]byte(entry.Metadata), &meta); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if got := meta["path"]; got != wantPath {
		t.Fatalf("metadata path = %q, want %q", got, wantPath)
	}
}

func TestEventNameForOp_IncludesChmod(t *testing.T) {
	if got := eventNameForOp(0); got != "" {
		t.Fatalf("eventNameForOp(0) = %q, want empty string", got)
	}
	if got := eventNameForOp(fsnotify.Chmod); got != "chmod" {
		t.Fatalf("eventNameForOp(chmod) = %q, want chmod", got)
	}
}

func waitForEntry(t *testing.T, out <-chan types.Entry) types.Entry {
	t.Helper()

	select {
	case entry := <-out:
		return entry
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for watcher event")
		return types.Entry{}
	}
}
