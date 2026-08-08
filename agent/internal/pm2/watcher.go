package pm2

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
)

const (
	defaultPollInterval = 15 * time.Second
	commandTimeout      = 10 * time.Second
)

// Settings contains the server-controlled PM2 source configuration.
// An empty process list means that every process returned by pm2 jlist is watched.
type Settings struct {
	mu        sync.RWMutex
	enabled   bool
	processes []string
}

func NewSettings(enabled bool, processes []string) *Settings {
	s := &Settings{}
	s.Set(enabled, processes)
	return s
}

func (s *Settings) Snapshot() (bool, []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled, append([]string(nil), s.processes...)
}

func (s *Settings) Set(enabled bool, processes []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	s.processes = append([]string(nil), processes...)
}

type process struct {
	PMID   int    `json:"pm_id"`
	Name   string `json:"name"`
	PID    int    `json:"pid"`
	PM2Env struct {
		Status           string `json:"status"`
		RestartTime      int    `json:"restart_time"`
		UnstableRestarts int    `json:"unstable_restarts"`
		PMUptime         int64  `json:"pm_uptime"`
		ExitCode         int    `json:"exit_code"`
	} `json:"pm2_env"`
}

type snapshot map[string]process

// Supported reports whether the configured PM2 executable is available.
func Supported() bool {
	_, err := exec.LookPath(binaryPath())
	return err == nil
}

func binaryPath() string {
	if configured := strings.TrimSpace(os.Getenv("PM2_BIN")); configured != "" {
		return configured
	}
	return "pm2"
}

// Watch polls pm2 jlist and emits only lifecycle transitions after the initial
// snapshot. The initial snapshot is deliberately a baseline so enabling the
// source does not flood the timeline with every already-running process.
func Watch(ctx context.Context, nodeName string, settings *Settings, out chan<- types.Entry) {
	watch(ctx, nodeName, settings, out, runJList, defaultPollInterval)
}

type jlistRunner func(context.Context) ([]process, error)

func watch(ctx context.Context, nodeName string, settings *Settings, out chan<- types.Entry, run jlistRunner, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var previous snapshot
	var previousProcesses []string
	initialized := false

	poll := func() {
		enabled, processes := settings.Snapshot()
		if !enabled {
			previous = nil
			previousProcesses = nil
			initialized = false
			return
		}
		if !initialized || !sameProcesses(previousProcesses, processes) {
			previous = nil
			initialized = false
			previousProcesses = append([]string(nil), processes...)
		}

		commandCtx, cancel := context.WithTimeout(ctx, commandTimeout)
		current, err := run(commandCtx)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("pm2 watcher: pm2 jlist failed: %v", err)
			}
			return
		}

		entries, next := transitions(nodeName, time.Now().UTC(), previous, current, initialized, processes)
		previous = next
		initialized = true
		for _, entry := range entries {
			select {
			case out <- entry:
			case <-ctx.Done():
				return
			}
		}
	}

	poll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}

func sameProcesses(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func runJList(ctx context.Context) ([]process, error) {
	output, err := exec.CommandContext(ctx, binaryPath(), "jlist").Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("execute %s jlist: %w", binaryPath(), err)
	}
	return parseJList(output)
}

func parseJList(data []byte) ([]process, error) {
	var processes []process
	if err := json.Unmarshal(data, &processes); err != nil {
		return nil, fmt.Errorf("decode jlist output: %w", err)
	}
	return processes, nil
}

func transitions(nodeName string, now time.Time, previous snapshot, current []process, initialized bool, allowed []string) ([]types.Entry, snapshot) {
	filtered := filterProcesses(current, allowed)
	next := make(snapshot, len(filtered))
	for _, process := range filtered {
		next[processKey(process)] = process
	}
	if !initialized {
		return nil, next
	}

	ordered := append([]process(nil), filtered...)
	sort.SliceStable(ordered, func(i, j int) bool { return processKey(ordered[i]) < processKey(ordered[j]) })
	entries := make([]types.Entry, 0, len(ordered))
	for _, process := range ordered {
		key := processKey(process)
		previousProcess, existed := previous[key]
		event := transitionEvent(previousProcess, process, existed)
		if event == "" {
			continue
		}
		metadata := map[string]any{
			"pm_id":             process.PMID,
			"pid":               process.PID,
			"status":            process.PM2Env.Status,
			"restart_time":      process.PM2Env.RestartTime,
			"unstable_restarts": process.PM2Env.UnstableRestarts,
		}
		if existed {
			metadata["previous_status"] = previousProcess.PM2Env.Status
		}
		metadataJSON, _ := json.Marshal(metadata)
		entries = append(entries, types.Entry{
			ID:        ulid.Make().String(),
			Timestamp: now,
			NodeName:  nodeName,
			Source:    "pm2",
			Service:   process.Name,
			Event:     event,
			Content:   fmt.Sprintf("PM2 process %s %s", process.Name, event),
			Metadata:  string(metadataJSON),
		})
	}
	return entries, next
}

func filterProcesses(processes []process, allowed []string) []process {
	if len(allowed) == 0 {
		return append([]process(nil), processes...)
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	filtered := make([]process, 0, len(processes))
	for _, process := range processes {
		if _, ok := allowedSet[process.Name]; ok {
			filtered = append(filtered, process)
		}
	}
	return filtered
}

func processKey(process process) string {
	if process.PMID >= 0 {
		return fmt.Sprintf("id:%d", process.PMID)
	}
	return "name:" + process.Name
}

func transitionEvent(previous, current process, existed bool) string {
	status := strings.ToLower(strings.TrimSpace(current.PM2Env.Status))
	previousStatus := strings.ToLower(strings.TrimSpace(previous.PM2Env.Status))
	if !existed {
		switch status {
		case "online":
			return "started"
		case "errored":
			return "failed"
		}
		return ""
	}
	if status == "online" && previousStatus != "online" {
		return "started"
	}
	if (status == "stopped" || status == "stopping") && previousStatus != "stopped" && previousStatus != "stopping" {
		return "stopped"
	}
	if status == "errored" && previousStatus != "errored" {
		return "failed"
	}
	if status == "online" && current.PM2Env.RestartTime > previous.PM2Env.RestartTime {
		return "restart"
	}
	return ""
}
