package files

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	if count := Watch(ctx, "node-1", []string{root}, nil, NewSettings(true), out); count == 0 {
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

	var meta map[string]any
	if err := json.Unmarshal([]byte(entry.Metadata), &meta); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if got := meta["path"]; got != target {
		t.Fatalf("metadata path = %q, want %q", got, target)
	}
	diff, _ := meta["diff"].(string)
	if !strings.Contains(diff, "-    image: lscr.io/linuxserver/sonarr\n") {
		t.Fatalf("diff missing old image line: %q", diff)
	}
	if !strings.Contains(diff, "+    image: lscr.io/linuxserver/sonarr:latest\n") {
		t.Fatalf("diff missing new image line: %q", diff)
	}
	if got := meta["diff_status"]; got != "included" {
		t.Fatalf("diff_status = %v, want included", got)
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
	if count := Watch(ctx, "node-1", []string{linkRoot}, nil, NewSettings(true), out); count == 0 {
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

	var meta map[string]any
	if err := json.Unmarshal([]byte(entry.Metadata), &meta); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if got := meta["path"]; got != wantPath {
		t.Fatalf("metadata path = %q, want %q", got, wantPath)
	}
}

func TestWatch_RedactsSensitiveDiffValues(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, ".env")
	if err := os.WriteFile(target, []byte("API_TOKEN=old-secret\nNORMAL=value\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan types.Entry, 4)
	if count := Watch(ctx, "node-1", []string{root}, nil, NewSettings(true), out); count == 0 {
		t.Fatal("expected watcher to register at least one directory")
	}

	if err := os.WriteFile(target, []byte("API_TOKEN=new-secret\nNORMAL=changed\n"), 0o644); err != nil {
		t.Fatalf("rewrite env file: %v", err)
	}

	entry := waitForEntry(t, out)
	var meta map[string]any
	if err := json.Unmarshal([]byte(entry.Metadata), &meta); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}

	diff, _ := meta["diff"].(string)
	if strings.Contains(diff, "old-secret") || strings.Contains(diff, "new-secret") {
		t.Fatalf("diff leaked secret value: %q", diff)
	}
	if !strings.Contains(diff, "API_TOKEN= [REDACTED]\n") {
		t.Fatalf("diff missing redacted token line: %q", diff)
	}
	if !strings.Contains(diff, "+NORMAL=changed\n") {
		t.Fatalf("diff missing non-sensitive change: %q", diff)
	}
}

func TestWatch_AllowsSensitiveDiffValuesWhenRedactionDisabled(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, ".env")
	if err := os.WriteFile(target, []byte("API_TOKEN=old-secret\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan types.Entry, 4)
	if count := Watch(ctx, "node-1", []string{root}, nil, NewSettings(false), out); count == 0 {
		t.Fatal("expected watcher to register at least one directory")
	}

	if err := os.WriteFile(target, []byte("API_TOKEN=new-secret\n"), 0o644); err != nil {
		t.Fatalf("rewrite env file: %v", err)
	}

	entry := waitForEntry(t, out)
	var meta map[string]any
	if err := json.Unmarshal([]byte(entry.Metadata), &meta); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}

	diff, _ := meta["diff"].(string)
	if !strings.Contains(diff, "-API_TOKEN=old-secret\n") || !strings.Contains(diff, "+API_TOKEN=new-secret\n") {
		t.Fatalf("diff missing raw secret values: %q", diff)
	}
	if got := meta["diff_redacted"]; got != false {
		t.Fatalf("diff_redacted = %v, want false", got)
	}
}

func TestEventNameForOp_IncludesChmod(t *testing.T) {
	tests := []struct {
		name string
		op   fsnotify.Op
		want string
	}{
		{name: "zero", op: 0, want: ""},
		{name: "write", op: fsnotify.Write, want: "write"},
		{name: "create", op: fsnotify.Create, want: "create"},
		{name: "remove", op: fsnotify.Remove, want: "remove"},
		{name: "rename", op: fsnotify.Rename, want: "rename"},
		{name: "chmod", op: fsnotify.Chmod, want: "chmod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eventNameForOp(tt.op); got != tt.want {
				t.Fatalf("eventNameForOp(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
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
