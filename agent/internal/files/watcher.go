package files

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

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

type watchRoot struct {
	configured string
	resolved   string
}

func Watch(ctx context.Context, nodeName string, rootPaths []string, ignorePatterns []string, out chan<- types.Entry) int {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("files watcher: failed to create fsnotify watcher: %v", err)
		return 0
	}

	roots := resolveWatchRoots(rootPaths)
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

	go runWatcher(ctx, nodeName, roots, ignorePatterns, watcher, out)
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
	p := filepath.Clean(path)
	for _, r := range roots {
		for _, candidate := range []string{r.configured, r.resolved} {
			candidate = filepath.Clean(candidate)
			if p == candidate || strings.HasPrefix(p, candidate+string(os.PathSeparator)) {
				return r.configured
			}
		}
	}
	return filepath.Dir(path)
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

func runWatcher(ctx context.Context, nodeName string, rootPaths []watchRoot, ignorePatterns []string, w *fsnotify.Watcher, out chan<- types.Entry) {
	defer func() {
		_ = w.Close()
	}()

	var mu sync.Mutex
	timers := make(map[string]*time.Timer)

	fire := func(path string, op fsnotify.Op) {
		eventPath := logicalPathFor(path, rootPaths)
		ext := strings.ToLower(filepath.Ext(eventPath))
		if !allowedExts[ext] {
			return
		}
		if isExcluded(eventPath, ignorePatterns) {
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

			meta, _ := json.Marshal(map[string]string{"path": eventPath, "op": event})
			entry := types.Entry{
				ID:        ulid.Make().String(),
				Timestamp: time.Now().UTC(),
				NodeName:  nodeName,
				Source:    "files",
				Service:   rootFor(path, rootPaths),
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
						newRoots := resolveWatchRoots([]string{event.Name})
						count := 0
						for _, root := range newRoots {
							added, err := addRecursive(w, root, ignorePatterns)
							if err != nil {
								log.Printf("files watcher: failed to register new dir %s: %v", event.Name, err)
								continue
							}
							count += added
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
