//go:build linux

package systemd

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	sdjournal "github.com/coreos/go-systemd/v22/sdjournal"
	"github.com/oklog/ulid/v2"

	"blackbox/shared/types"
)

const logCaptureLines = 50
const oomMessageSubstring = "Out of memory"

var (
	cachedJournal     *sdjournal.Journal
	cachedJournalOnce sync.Once
	cachedJournalErr  error
	cachedJournalMu   sync.Mutex
)

// watch is the inner loop — opens the journal and emits entries until ctx is done or an error occurs.
func watch(ctx context.Context, nodeName string, settings *Settings, out chan<- types.Entry) error {
	j, err := sdjournal.NewJournal()
	if err != nil {
		return err
	}
	defer func() { _ = j.Close() }()

	if err := j.SeekTail(); err != nil {
		return err
	}
	if _, err := j.Previous(); err != nil {
		return err
	}

	unitStates := make(map[string]string)
	deactivatingEmitted := make(map[string]bool)
	var lastUnits []string

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		units := settings.Units()

		if !stringSlicesEqual(units, lastUnits) {
			if err := j.FlushMatches(); err != nil {
				return err
			}
			for _, unit := range units {
				if err := j.AddMatch("_SYSTEMD_UNIT=" + unit); err != nil {
					return err
				}
				if err := j.AddDisjunction(); err != nil {
					return err
				}
			}
			if err := j.AddMatch("SYSLOG_FACILITY=0"); err != nil {
				return err
			}
			lastUnits = units
		}

		n, err := j.Next()
		if err != nil {
			return err
		}
		if n == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		entry, err := j.GetEntry()
		if err != nil {
			log.Printf("systemd watcher: get entry: %v", err)
			continue
		}

		unit := entry.Fields["_SYSTEMD_UNIT"]
		message := entry.Fields["MESSAGE"]
		facility := entry.Fields["SYSLOG_FACILITY"]

		if facility == "0" && strings.Contains(message, oomMessageSubstring) {
			out <- buildOOMEntry(nodeName, message, entry.RealtimeTimestamp)
			continue
		}

		if unit == "" {
			continue
		}

		currState := entry.Fields["ACTIVE_STATE"]
		if currState == "" {
			continue
		}

		prevState := unitStates[unit]
		if prevState == currState {
			continue
		}
		unitStates[unit] = currState

		event := mapTransition(prevState, currState)
		if event == "" {
			continue
		}

		if event == "stopped" && currState == "inactive" && deactivatingEmitted[unit] {
			deactivatingEmitted[unit] = false
			continue
		}
		if event == "stopped" && currState == "deactivating" {
			deactivatingEmitted[unit] = true
		} else {
			deactivatingEmitted[unit] = false
		}

		ts := time.Unix(0, int64(entry.RealtimeTimestamp)*int64(time.Microsecond)).UTC()

		meta := map[string]interface{}{
			"unit":           unit,
			"previous_state": prevState,
		}

		if event == "failed" {
			if snippet, err := captureUnitLogs(unit); err != nil {
				log.Printf("systemd watcher: capture logs for %s: %v", unit, err)
			} else if len(snippet) > 0 {
				meta["log_snippet"] = snippet
			}
		}

		metaBytes, _ := json.Marshal(meta)
		out <- types.Entry{
			ID:        ulid.Make().String(),
			Timestamp: ts,
			NodeName:  nodeName,
			Source:    "systemd",
			Service:   unit,
			Event:     event,
			Content:   unit + " " + event,
			Metadata:  string(metaBytes),
		}
	}
}

func buildOOMEntry(nodeName, message string, realtimeMicros uint64) types.Entry {
	ts := time.Unix(0, int64(realtimeMicros)*int64(time.Microsecond)).UTC()

	killedProcess := ""
	pid := ""
	parts := strings.Fields(message)
	for i, p := range parts {
		if p == "process" && i+1 < len(parts) {
			pid = parts[i+1]
		}
		if p == "Killed" && i+2 < len(parts) {
			candidate := parts[i+2]
			killedProcess = strings.Trim(candidate, "()")
		}
	}

	meta := map[string]interface{}{
		"killed_process": killedProcess,
		"pid":            pid,
	}
	metaBytes, _ := json.Marshal(meta)

	content := "OOM kill"
	if killedProcess != "" {
		content = "OOM kill: " + killedProcess
	}

	return types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: ts,
		NodeName:  nodeName,
		Source:    "systemd",
		Service:   "kernel",
		Event:     "oom_kill",
		Content:   content,
		Metadata:  string(metaBytes),
	}
}

func captureUnitLogs(unit string) ([]string, error) {
	j, err := journalForCapture()
	if err != nil {
		return nil, err
	}

	cachedJournalMu.Lock()
	defer cachedJournalMu.Unlock()

	if err := j.FlushMatches(); err != nil {
		return nil, err
	}
	defer func() {
		if err := j.FlushMatches(); err != nil {
			log.Printf("systemd watcher: flush journal matches: %v", err)
		}
	}()
	if err := j.AddMatch("_SYSTEMD_UNIT=" + unit); err != nil {
		return nil, err
	}

	if err := j.SeekTail(); err != nil {
		return nil, err
	}

	var lines []string
	for i := 0; i < logCaptureLines; i++ {
		n, err := j.Previous()
		if err != nil || n == 0 {
			break
		}
		entry, err := j.GetEntry()
		if err != nil {
			break
		}
		if msg := entry.Fields["MESSAGE"]; msg != "" {
			lines = append(lines, msg)
		}
	}

	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines, nil
}

func journalForCapture() (*sdjournal.Journal, error) {
	cachedJournalOnce.Do(func() {
		cachedJournal, cachedJournalErr = sdjournal.NewJournal()
	})
	if cachedJournalErr != nil {
		return nil, cachedJournalErr
	}
	if cachedJournal == nil {
		return nil, errors.New("systemd journal not initialized")
	}
	return cachedJournal, nil
}

func closeCachedJournal() {
	cachedJournalMu.Lock()
	defer cachedJournalMu.Unlock()
	if cachedJournal != nil {
		_ = cachedJournal.Close()
		cachedJournal = nil
	}
}
