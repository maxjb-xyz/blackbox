package files

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/fsnotify/fsnotify"
	"github.com/oklog/ulid/v2"

	"blackbox/shared/types"
)

var (
	allowedExts = map[string]bool{
		".yaml": true, ".yml": true, ".conf": true,
		".env": true, ".json": true, ".ini": true,
	}
	defaultExcludes = []string{".git", "node_modules", "cache", "logs"}
)

const debounceDelay = 500 * time.Millisecond

const (
	maxTrackedFileBytes = 64 << 10
	maxDiffLines        = 1600
	maxDiffBytes        = 32 << 10
	diffContextLines    = 2
)

type watchRoot struct {
	configured string
	resolved   string
}

type Settings struct {
	redactSecrets atomic.Bool
}

type snapshotStore struct {
	mu    sync.Mutex
	files map[string]string
}

type fileSnapshot struct {
	content string
	hash    string
	size    int
	status  string
}

type diffOp struct {
	kind byte
	line string
}

func NewSettings(redactSecrets bool) *Settings {
	s := &Settings{}
	s.SetRedactSecrets(redactSecrets)
	return s
}

func (s *Settings) RedactSecrets() bool {
	if s == nil {
		return true
	}
	return s.redactSecrets.Load()
}

func (s *Settings) SetRedactSecrets(enabled bool) {
	if s == nil {
		return
	}
	s.redactSecrets.Store(enabled)
}

func Watch(ctx context.Context, nodeName string, rootPaths []string, ignorePatterns []string, settings *Settings, out chan<- types.Entry) int {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("files watcher: failed to create fsnotify watcher: %v", err)
		return 0
	}

	roots := resolveWatchRoots(rootPaths)
	snapshots := primeSnapshots(roots, ignorePatterns)
	watchCount := 0
	for _, root := range roots {
		count, err := addRecursive(watcher, root, ignorePatterns)
		if err != nil {
			log.Printf("files watcher: failed to register root %s: %v", root.configured, err)
			continue
		}
		watchCount += count
		log.Printf("files watcher: registered %d directories for %s", count, root.configured)
	}

	go runWatcher(ctx, nodeName, roots, ignorePatterns, watcher, snapshots, settings, out)
	return watchCount
}

func resolveWatchRoots(rootPaths []string) []watchRoot {
	roots := make([]watchRoot, 0, len(rootPaths))
	seen := make(map[string]struct{}, len(rootPaths))
	for _, raw := range rootPaths {
		configured := filepath.Clean(strings.TrimSpace(raw))
		if configured == "." || configured == "" {
			continue
		}
		resolved := configured
		if target, err := filepath.EvalSymlinks(configured); err == nil {
			resolved = filepath.Clean(target)
			if resolved != configured {
				log.Printf("files watcher: resolved %s -> %s", configured, resolved)
			}
		}
		key := configured + "\x00" + resolved
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		roots = append(roots, watchRoot{configured: configured, resolved: resolved})
	}

	sort.Slice(roots, func(i, j int) bool {
		left := max(len(roots[i].configured), len(roots[i].resolved))
		right := max(len(roots[j].configured), len(roots[j].resolved))
		if left == right {
			return roots[i].configured < roots[j].configured
		}
		return left > right
	})
	return roots
}

func addRecursive(w *fsnotify.Watcher, root watchRoot, ignorePatterns []string) (int, error) {
	info, err := os.Stat(root.resolved)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("resolved root %s is not a directory", root.resolved)
	}

	count := 0
	if err := filepath.Walk(root.resolved, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if isExcluded(path, ignorePatterns) {
				return filepath.SkipDir
			}
			if err := w.Add(path); err != nil {
				log.Printf("files watcher: failed to watch %s: %v", path, err)
			} else {
				count++
			}
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(path)
			if err == nil {
				if tInfo, err := os.Stat(target); err == nil && !tInfo.IsDir() {
					if err := w.Add(target); err != nil {
						log.Printf("files watcher: failed to watch symlink target %s: %v", target, err)
					} else {
						count++
					}
				}
			}
		}
		return nil
	}); err != nil {
		return count, err
	}
	return count, nil
}

func isExcluded(path string, ignorePatterns []string) bool {
	base := filepath.Base(path)
	for _, excl := range defaultExcludes {
		if base == excl {
			return true
		}
	}
	for _, pattern := range ignorePatterns {
		if pattern == "" {
			continue
		}
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
}

func rootFor(path string, roots []watchRoot) string {
	p := slashClean(path)
	for _, r := range roots {
		for _, candidate := range []string{r.configured, r.resolved} {
			candidate = slashClean(candidate)
			if p == candidate || strings.HasPrefix(p, candidate+"/") {
				return slashClean(r.configured)
			}
		}
	}
	return slashClean(filepath.Dir(path))
}

var collectionDirs = map[string]bool{
	"stacks": true, "docker": true, "containers": true,
	"compose": true, "apps": true, "services": true,
}

// serviceFromPath extracts a service name from a file path by stripping
// common config directory prefixes. Falls back to the watch root path.
func serviceFromPath(filePath string, roots []watchRoot) string {
	logical := slashClean(logicalPathFor(filePath, roots))

	prefixes := []string{"/etc/", "/opt/", "/var/lib/", "/srv/"}
	for _, prefix := range prefixes {
		base := strings.TrimSuffix(prefix, "/")
		if logical == base || logical == prefix {
			return ""
		}
		if strings.HasPrefix(logical, prefix) {
			rest := strings.TrimPrefix(logical, prefix)
			if rest == "" || rest == "/" {
				return ""
			}
			parts := strings.SplitN(rest, "/", 3)
			// Require 3 parts so parts[1] is a real directory, not a bare filename
			// (e.g. /opt/stacks/.env splits to ["stacks", ".env"] — len 2 — and
			// we should NOT return ".env" as the service name).
			if len(parts) >= 3 && collectionDirs[parts[0]] && parts[1] != "" {
				return parts[1]
			}
			// Don't return collection dir names (stacks, docker, …) as the service.
			// A file sitting directly inside a collection dir has no determinable
			// service; fall through to rootFor.
			if parts[0] != "" && !collectionDirs[parts[0]] {
				return parts[0]
			}
		}
	}

	// Handle /home/<user>/<collection>/<service>/...
	if strings.HasPrefix(logical, "/home/") {
		rest := strings.TrimPrefix(logical, "/home/")
		parts := strings.SplitN(rest, "/", 4) // user / dir / service / file
		// Require 4 parts: user, collection dir, service dir, and at least one
		// more component (the file). len==3 means the file sits directly inside
		// the collection dir (/home/user/stacks/.env) and we can't identify a
		// service from it.
		if len(parts) >= 4 && collectionDirs[parts[1]] && parts[2] != "" {
			return parts[2]
		}
	}

	return rootFor(filePath, roots)
}

func slashClean(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func logicalPathFor(path string, roots []watchRoot) string {
	p := filepath.Clean(path)
	for _, r := range roots {
		candidate := filepath.Clean(r.resolved)
		if p == candidate {
			return r.configured
		}
		prefix := candidate + string(os.PathSeparator)
		if strings.HasPrefix(p, prefix) {
			suffix := strings.TrimPrefix(p, prefix)
			return filepath.Join(r.configured, suffix)
		}
	}
	return p
}

func eventNameForOp(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Write != 0:
		return "write"
	case op&fsnotify.Create != 0:
		return "create"
	case op&fsnotify.Remove != 0:
		return "remove"
	case op&fsnotify.Rename != 0:
		return "rename"
	case op&fsnotify.Chmod != 0:
		return "chmod"
	default:
		return ""
	}
}

func newSnapshotStore() *snapshotStore {
	return &snapshotStore{files: make(map[string]string)}
}

func (s *snapshotStore) Get(path string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, ok := s.files[path]
	return content, ok
}

func (s *snapshotStore) Set(path, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[path] = content
}

func (s *snapshotStore) Delete(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, path)
}

func primeSnapshots(roots []watchRoot, ignorePatterns []string) *snapshotStore {
	store := newSnapshotStore()
	for _, root := range roots {
		if err := filepath.Walk(root.resolved, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			logicalPath := logicalPathFor(path, roots)
			if info.IsDir() {
				if isExcluded(logicalPath, ignorePatterns) {
					return filepath.SkipDir
				}
				return nil
			}
			if !shouldTrackFile(logicalPath, ignorePatterns) {
				return nil
			}
			snapshot := readSnapshot(path)
			if snapshot.status == "ok" {
				store.Set(logicalPath, snapshot.content)
			}
			return nil
		}); err != nil {
			log.Printf("files watcher: failed to prime snapshots for %s: %v", root.configured, err)
		}
	}
	return store
}

func shouldTrackFile(path string, ignorePatterns []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if !allowedExts[ext] {
		return false
	}
	return !isExcluded(path, ignorePatterns)
}

func readSnapshot(path string) fileSnapshot {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSnapshot{status: "missing"}
		}
		return fileSnapshot{status: "read_error"}
	}
	defer func() {
		_ = file.Close()
	}()

	info, err := file.Stat()
	if err != nil {
		return fileSnapshot{status: "read_error"}
	}
	if info.IsDir() {
		return fileSnapshot{status: "missing"}
	}

	reader := bufio.NewReader(io.LimitReader(file, maxTrackedFileBytes+1))
	data, err := io.ReadAll(reader)
	if err != nil {
		return fileSnapshot{status: "read_error"}
	}
	if len(data) > maxTrackedFileBytes {
		return fileSnapshot{status: "too_large", size: len(data)}
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') {
		return fileSnapshot{status: "binary", size: len(data)}
	}

	content := normalizeContent(string(data))
	return fileSnapshot{
		content: content,
		hash:    sha256Hex(content),
		size:    len(content),
		status:  "ok",
	}
}

func normalizeContent(content string) string {
	return strings.ReplaceAll(content, "\r\n", "\n")
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func redactSensitiveContent(content string) string {
	lines := strings.SplitAfter(content, "\n")
	for i, line := range lines {
		lines[i] = redactSensitiveLine(line)
	}
	return strings.Join(lines, "")
}

func redactSensitiveLine(line string) string {
	if line == "" {
		return line
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return line
	}
	for _, sep := range []string{":", "="} {
		idx := strings.Index(line, sep)
		if idx <= 0 {
			continue
		}
		key := normalizeSensitiveKey(line[:idx])
		if !looksSensitiveKey(key) {
			continue
		}
		suffix := ""
		if strings.HasSuffix(strings.TrimRight(line, "\r\n"), ",") {
			suffix = ","
		}
		lineEnding := ""
		if strings.HasSuffix(line, "\n") {
			lineEnding = "\n"
		}
		return line[:idx+1] + " [REDACTED]" + suffix + lineEnding
	}
	return line
}

func normalizeSensitiveKey(raw string) string {
	key := strings.TrimSpace(raw)
	key = strings.Trim(key, `"'`)
	replacer := strings.NewReplacer("-", "_", ".", "_", " ", "_")
	return strings.ToLower(replacer.Replace(key))
}

func looksSensitiveKey(key string) bool {
	for _, marker := range []string{
		"password",
		"passwd",
		"secret",
		"token",
		"api_key",
		"apikey",
		"access_key",
		"secret_key",
		"client_secret",
		"private_key",
		"auth_token",
		"bearer_token",
	} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func buildMetadata(actualPath, logicalPath, event string, snapshots *snapshotStore, settings *Settings) map[string]any {
	meta := map[string]any{
		"path": logicalPath,
		"op":   event,
	}

	before, hadBefore := snapshots.Get(logicalPath)
	after := readSnapshot(actualPath)
	switch after.status {
	case "ok":
		snapshots.Set(logicalPath, after.content)
	case "missing":
		snapshots.Delete(logicalPath)
	case "too_large", "binary", "read_error":
		if event == "remove" || event == "rename" {
			snapshots.Delete(logicalPath)
		}
	}

	if hadBefore {
		meta["before_sha256"] = sha256Hex(before)
		meta["before_size"] = len(before)
	}
	if after.status == "ok" {
		meta["after_sha256"] = after.hash
		meta["after_size"] = after.size
	} else {
		meta["after_state"] = after.status
	}

	diff, status := buildDiff(before, hadBefore, after, settings.RedactSecrets())
	meta["diff_status"] = status
	if diff != "" {
		meta["diff"] = diff
	}
	meta["diff_redacted"] = settings.RedactSecrets()
	return meta
}

func buildDiff(before string, hadBefore bool, after fileSnapshot, redactSecrets bool) (string, string) {
	switch after.status {
	case "too_large", "binary", "read_error":
		return "", "skipped_" + after.status
	}

	afterContent := ""
	if after.status == "ok" {
		afterContent = after.content
	}

	if !hadBefore {
		if after.status != "ok" {
			return "", "no_baseline"
		}
	}

	if hadBefore && after.status == "ok" && before == afterContent {
		return "", "unchanged"
	}

	if redactSecrets {
		before = redactSensitiveContent(before)
		afterContent = redactSensitiveContent(afterContent)
	}

	beforeLines := splitDiffLines(before)
	afterLines := splitDiffLines(afterContent)
	if len(beforeLines)+len(afterLines) > maxDiffLines {
		return "", "skipped_too_many_lines"
	}

	ops := computeDiff(beforeLines, afterLines)
	hasChanges := false
	for _, op := range ops {
		if op.kind != ' ' {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return "", "unchanged"
	}

	diff := renderDiff(ops)
	if diff == "" {
		return "", "unchanged"
	}
	return diff, "included"
}

func splitDiffLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.SplitAfter(content, "\n")
}

func computeDiff(before, after []string) []diffOp {
	dp := make([][]int, len(before)+1)
	for i := range dp {
		dp[i] = make([]int, len(after)+1)
	}
	for i := len(before) - 1; i >= 0; i-- {
		for j := len(after) - 1; j >= 0; j-- {
			if before[i] == after[j] {
				dp[i][j] = dp[i+1][j+1] + 1
				continue
			}
			dp[i][j] = max(dp[i+1][j], dp[i][j+1])
		}
	}

	ops := make([]diffOp, 0, len(before)+len(after))
	for i, j := 0, 0; i < len(before) || j < len(after); {
		switch {
		case i < len(before) && j < len(after) && before[i] == after[j]:
			ops = append(ops, diffOp{kind: ' ', line: before[i]})
			i++
			j++
		case j < len(after) && (i == len(before) || dp[i][j+1] >= dp[i+1][j]):
			ops = append(ops, diffOp{kind: '+', line: after[j]})
			j++
		default:
			ops = append(ops, diffOp{kind: '-', line: before[i]})
			i++
		}
	}
	return ops
}

func renderDiff(ops []diffOp) string {
	var changed []int
	for i, op := range ops {
		if op.kind != ' ' {
			changed = append(changed, i)
		}
	}
	if len(changed) == 0 {
		return ""
	}

	visible := make([]bool, len(ops))
	for _, idx := range changed {
		start := max(0, idx-diffContextLines)
		end := min(len(ops), idx+diffContextLines+1)
		for i := start; i < end; i++ {
			visible[i] = true
		}
	}

	var b strings.Builder
	b.WriteString("--- before\n+++ after\n")
	hiddenRun := 0
	for i, op := range ops {
		if !visible[i] {
			hiddenRun++
			continue
		}
		if hiddenRun > 0 {
			fmt.Fprintf(&b, "@@ %d unchanged line(s) @@\n", hiddenRun)
			hiddenRun = 0
		}
		b.WriteByte(op.kind)
		b.WriteString(op.line)
		if !strings.HasSuffix(op.line, "\n") {
			b.WriteString("\n")
		}
	}
	if hiddenRun > 0 {
		fmt.Fprintf(&b, "@@ %d unchanged line(s) @@\n", hiddenRun)
	}

	diff := b.String()
	if len(diff) > maxDiffBytes {
		diff = diff[:maxDiffBytes]
		if cut := strings.LastIndex(diff, "\n"); cut > 0 {
			diff = diff[:cut+1]
		}
		diff += "@@ diff truncated @@\n"
	}
	return diff
}

func runWatcher(ctx context.Context, nodeName string, rootPaths []watchRoot, ignorePatterns []string, w *fsnotify.Watcher, snapshots *snapshotStore, settings *Settings, out chan<- types.Entry) {
	defer func() {
		_ = w.Close()
	}()

	var mu sync.Mutex
	timers := make(map[string]*time.Timer)

	fire := func(path string, op fsnotify.Op) {
		eventPath := logicalPathFor(path, rootPaths)
		if !shouldTrackFile(eventPath, ignorePatterns) {
			return
		}
		mu.Lock()
		if t, ok := timers[eventPath]; ok {
			t.Stop()
		}
		timers[eventPath] = time.AfterFunc(debounceDelay, func() {
			mu.Lock()
			delete(timers, eventPath)
			mu.Unlock()

			event := eventNameForOp(op)
			if event == "" {
				return
			}

			meta, _ := json.Marshal(buildMetadata(path, eventPath, event, snapshots, settings))
			entry := types.Entry{
				ID:        ulid.Make().String(),
				Timestamp: time.Now().UTC(),
				NodeName:  nodeName,
				Source:    "files",
				Service:   serviceFromPath(path, rootPaths),
				Event:     event,
				Content:   fmt.Sprintf("file %s: %s", event, eventPath),
				Metadata:  string(meta),
			}
			select {
			case <-ctx.Done():
				return
			case out <- entry:
			}
		})
		mu.Unlock()
	}

	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, t := range timers {
				t.Stop()
			}
			timers = make(map[string]*time.Timer)
			mu.Unlock()
			return
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if !isExcluded(event.Name, ignorePatterns) {
						count := 0
						if newRoots := resolveWatchRoots([]string{event.Name}); len(newRoots) > 0 {
							added, err := addRecursive(w, newRoots[0], ignorePatterns)
							if err != nil {
								log.Printf("files watcher: failed to register new dir %s: %v", event.Name, err)
							} else {
								count = added
							}
						}
						if count == 0 {
							log.Printf("files watcher: no subdirectories added for new dir %s (may be empty or all excluded)", event.Name)
						}
					}
					continue
				}
			}
			fire(event.Name, event.Op)
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Printf("files watcher: error: %v", err)
		}
	}
}
