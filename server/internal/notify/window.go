package notify

import (
	"strconv"
	"strings"
	"time"
)

// Decision values recorded in NotificationLog.Decision.
const (
	decisionSent         = "sent"
	decisionHeld         = "held"
	decisionDroppedRate  = "dropped_rate"
	decisionDroppedQuiet = "dropped_quiet"
	decisionFailed       = "failed"
	decisionDigest       = "digest"
)

// parseHHMM converts "HH:MM" (24h) to minutes-since-midnight. ok is false on
// any malformed input.
func parseHHMM(s string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// inWindowMinutes reports whether nowMin (minutes-since-midnight) falls in
// [start, end). A window where start > end wraps past midnight. A zero-length
// or malformed window is never "in".
func inWindowMinutes(nowMin int, start, end string) bool {
	s, ok1 := parseHHMM(start)
	e, ok2 := parseHHMM(end)
	if !ok1 || !ok2 || s == e {
		return false
	}
	if s < e {
		return nowMin >= s && nowMin < e
	}
	return nowMin >= s || nowMin < e
}

// inQuietWindow evaluates the window for a wall-clock time in its location.
func inQuietWindow(now time.Time, start, end string) bool {
	return inWindowMinutes(now.Hour()*60+now.Minute(), start, end)
}

// rateWindow maps a unit string to its sliding-window duration.
func rateWindow(unit string) time.Duration {
	if unit == "day" {
		return 24 * time.Hour
	}
	return time.Hour
}

// appendNote appends a non-empty note to a message body on its own paragraph.
func appendNote(body, note string) string {
	if note == "" {
		return body
	}
	if body == "" {
		return note
	}
	return body + "\n\n" + note
}
