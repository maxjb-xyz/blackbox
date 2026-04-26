package files

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"blackbox/shared/types"
	"github.com/fsnotify/fsnotify"
)

func TestServiceFromPath(t *testing.T) {
	roots := []watchRoot{
		{configured: "/etc/nginx", resolved: "/etc/nginx"},
		{configured: "/opt/myapp", resolved: "/opt/myapp"},
		{configured: "/home/user/docker/redis", resolved: "/home/user/docker/redis"},
	}
	cases := []struct {
		name string
		path string
		want string
	}{
		{"etc child", "/etc/nginx/sites-available/foo.conf", "nginx"},
		{"opt child", "/opt/myapp/config.yml", "myapp"},
		{"docker home child", "/home/user/docker/redis/redis.conf", "redis"},
		{"opt stacks collection", "/opt/stacks/my-app/.env", "my-app"},
		{"opt docker collection", "/opt/docker/my-app/compose.yml", "my-app"},
		{"srv stacks collection", "/srv/stacks/my-app/config.yml", "my-app"},
		{"home stacks collection", "/home/user/stacks/my-app/.env", "my-app"},
		{"etc exact service", "/etc/nginx", "nginx"},
		{"opt exact service", "/opt/myapp", "myapp"},
		{"var lib child", "/var/lib/foo/bar.conf", "foo"},
		{"unmatched path falls back to root dir", "/custom/path/config.yml", "/custom/path"},
		{"empty path falls back deterministically", "", "."},
		{"trailing slash still resolves service", "/etc/nginx/", "nginx"},
		{"exact prefix returns empty service", "/etc", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := serviceFromPath(tc.path, roots)
			if got != tc.want {
				t.Fatalf("serviceFromPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

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
	if entry.Service != filepath.ToSlash(filepath.Clean(root)) {
		t.Fatalf("entry service = %q, want %q", entry.Service, filepath.ToSlash(filepath.Clean(root)))
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
	if runtime.GOOS == "windows" {
		t.Skip("creating directory symlinks requires elevated Windows privileges in this environment")
	}

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
	if entry.Service != filepath.ToSlash(filepath.Clean(linkRoot)) {
		t.Fatalf("entry service = %q, want %q", entry.Service, filepath.ToSlash(filepath.Clean(linkRoot)))
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

func TestAddRecursive_SkipsUnreadableChildDirectory(t *testing.T) {
	skipIfUnreadableDirsRemainReadable(t)

	root := t.TempDir()
	readable := filepath.Join(root, "readable")
	unreadable := filepath.Join(root, "unreadable")
	if err := os.MkdirAll(readable, 0o755); err != nil {
		t.Fatalf("mkdir readable dir: %v", err)
	}
	if err := os.MkdirAll(unreadable, 0o755); err != nil {
		t.Fatalf("mkdir unreadable dir: %v", err)
	}
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatalf("chmod unreadable dir: %v", err)
	}
	defer func() {
		_ = os.Chmod(unreadable, 0o755)
	}()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new fsnotify watcher: %v", err)
	}
	defer func() {
		_ = watcher.Close()
	}()

	count, err := addRecursive(watcher, watchRoot{configured: root, resolved: root}, nil)
	if err != nil {
		t.Fatalf("addRecursive returned error for unreadable child: %v", err)
	}
	if count == 0 {
		t.Fatal("expected addRecursive to register readable directories")
	}
}

func TestPrimeSnapshots_SkipsUnreadableChildDirectory(t *testing.T) {
	skipIfUnreadableDirsRemainReadable(t)

	root := t.TempDir()
	unreadable := filepath.Join(root, "aaa-unreadable")
	if err := os.MkdirAll(unreadable, 0o755); err != nil {
		t.Fatalf("mkdir unreadable dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unreadable, "secret.env"), []byte("TOKEN=hidden\n"), 0o644); err != nil {
		t.Fatalf("write unreadable child file: %v", err)
	}
	visible := filepath.Join(root, "zzz-visible.yml")
	if err := os.WriteFile(visible, []byte("services:\n  app:\n    image: example/app\n"), 0o644); err != nil {
		t.Fatalf("write visible file: %v", err)
	}
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatalf("chmod unreadable dir: %v", err)
	}
	defer func() {
		_ = os.Chmod(unreadable, 0o755)
	}()

	store := primeSnapshots([]watchRoot{{configured: root, resolved: root}}, nil)
	if _, ok := store.Get(visible); !ok {
		t.Fatal("expected snapshot priming to continue after unreadable child directory")
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

func TestWatch_SuppressesEventsWhenDisabled(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config.yml")
	if err := os.WriteFile(target, []byte("key: old\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := NewSettings(true)
	settings.SetEnabled(false)

	out := make(chan types.Entry, 4)
	if count := Watch(ctx, "node-1", []string{root}, nil, settings, out); count == 0 {
		t.Fatal("expected watcher to register at least one directory")
	}

	if err := os.WriteFile(target, []byte("key: new\n"), 0o644); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}

	select {
	case entry := <-out:
		t.Fatalf("expected no file events while disabled, got %+v", entry)
	case <-time.After(1200 * time.Millisecond):
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

func skipIfUnreadableDirsRemainReadable(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission checks are not portable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can still traverse directories with permission bits cleared")
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
