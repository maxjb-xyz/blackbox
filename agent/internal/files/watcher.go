package files

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
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

func Watch(ctx context.Context, nodeName string, rootPaths []string, ignorePatterns []string, out chan<- types.Entry) int {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("files watcher: failed to create fsnotify watcher: %v", err)
		return 0
	}

	watchCount := 0
	for _, root := range rootPaths {
		watchCount += addRecursive(watcher, root, ignorePatterns)
	}

	go runWatcher(ctx, nodeName, rootPaths, ignorePatterns, watcher, out)
	return watchCount
}

func addRecursive(w *fsnotify.Watcher, root string, ignorePatterns []string) int {
	count := 0
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
		log.Printf("files watcher: walk error on %s: %v", root, err)
	}
	return count
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

func rootFor(path string, roots []string) string {
	p := filepath.Clean(path)
	for _, r := range roots {
		r = filepath.Clean(r)
		if p == r || strings.HasPrefix(p, r+string(os.PathSeparator)) {
			return r
		}
	}
	return filepath.Dir(path)
}

func runWatcher(ctx context.Context, nodeName string, rootPaths []string, ignorePatterns []string, w *fsnotify.Watcher, out chan<- types.Entry) {
	defer w.Close()

	var mu sync.Mutex
	timers := make(map[string]*time.Timer)

	fire := func(path string, op fsnotify.Op) {
		ext := strings.ToLower(filepath.Ext(path))
		if !allowedExts[ext] {
			return
		}
		if isExcluded(path, ignorePatterns) {
			return
		}
		mu.Lock()
		if t, ok := timers[path]; ok {
			t.Stop()
		}
		timers[path] = time.AfterFunc(debounceDelay, func() {
			mu.Lock()
			delete(timers, path)
			mu.Unlock()

			var event string
			switch {
			case op&fsnotify.Write != 0:
				event = "write"
			case op&fsnotify.Create != 0:
				event = "create"
			case op&fsnotify.Remove != 0:
				event = "remove"
			case op&fsnotify.Rename != 0:
				event = "rename"
			default:
				return
			}

			meta, _ := json.Marshal(map[string]string{"path": path, "op": event})
			entry := types.Entry{
				ID:        ulid.Make().String(),
				Timestamp: time.Now().UTC(),
				NodeName:  nodeName,
				Source:    "files",
				Service:   rootFor(path, rootPaths),
				Event:     event,
				Content:   fmt.Sprintf("file %s: %s", event, path),
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
						count := addRecursive(w, event.Name, ignorePatterns)
						if count == 0 {
							log.Printf("files watcher: failed to add new dir %s or its subdirectories", event.Name)
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
