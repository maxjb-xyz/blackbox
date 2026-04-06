package correlation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"blackbox/shared/types"
	"gorm.io/gorm"
)

// CauseCandidate is a scored entry that may explain an incident.
type CauseCandidate struct {
	Entry  *types.Entry
	Score  int
	Reason string
}

const MinCauseScore = 40

var eventWindows = map[string]time.Duration{
	"die":      60 * time.Second,
	"restart":  60 * time.Second,
	"stop":     120 * time.Second,
	"failed":   120 * time.Second,
	"stopped":  120 * time.Second,
	"oom_kill": 120 * time.Second,
	"update":   300 * time.Second,
	"pull":     300 * time.Second,
	"write":    120 * time.Second,
	"create":   120 * time.Second,
}

const maxLookbackWindow = 300 * time.Second

// ScoreCauses returns all candidate cause entries above MinCauseScore,
// ordered by score descending. The caller should apply ApplyNodeBonus
// once the trigger node is known.
func ScoreCauses(db *gorm.DB, services []string, at time.Time) ([]CauseCandidate, error) {
	if len(services) == 0 {
		return []CauseCandidate{}, nil
	}
	windowStart := at.Add(-maxLookbackWindow)

	var candidates []types.Entry
	err := db.Where(
		"service IN ? AND timestamp BETWEEN ? AND ? AND NOT (source = ? AND event IN ?)",
		services, windowStart, at, "webhook", []string{"down", "up"},
	).Order("timestamp DESC").Find(&candidates).Error
	if err != nil {
		return nil, err
	}

	var results []CauseCandidate
	for i := range candidates {
		e := &candidates[i]
		window, ok := eventWindows[e.Event]
		if !ok {
			continue
		}
		if at.Sub(e.Timestamp) > window {
			continue
		}
		base := baseScore(e)
		if base == 0 {
			continue
		}
		bonus := 0
		if hasLogSnippet(e) {
			bonus += 10
		}
		score := base + bonus
		if score < MinCauseScore {
			continue
		}
		elapsed := int(at.Sub(e.Timestamp).Seconds())
		results = append(results, CauseCandidate{
			Entry:  e,
			Score:  score,
			Reason: fmt.Sprintf("%s %s %ds before trigger", e.Source, e.Event, elapsed),
		})
	}

	sort.Slice(results, func(i, j int) bool { return causeCandidateLess(results[i], results[j]) })
	return results, nil
}

// ApplyNodeBonus adds +20 to candidates from the same node as triggerNode
// and re-sorts by score descending.
func ApplyNodeBonus(candidates []CauseCandidate, triggerNode string) {
	if triggerNode != "" {
		for i := range candidates {
			if candidates[i].Entry.NodeName == triggerNode {
				candidates[i].Score += 20
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return causeCandidateLess(candidates[i], candidates[j]) })
}

func causeCandidateLess(left, right CauseCandidate) bool {
	if left.Score != right.Score {
		return left.Score > right.Score
	}
	if left.Entry != nil && right.Entry != nil && !left.Entry.Timestamp.Equal(right.Entry.Timestamp) {
		return left.Entry.Timestamp.After(right.Entry.Timestamp)
	}
	leftID := ""
	if left.Entry != nil {
		leftID = left.Entry.ID
	}
	rightID := ""
	if right.Entry != nil {
		rightID = right.Entry.ID
	}
	return leftID < rightID
}

func baseScore(e *types.Entry) int {
	switch e.Event {
	case "die":
		if ec := extractExitCode(e); ec != "" && ec != "0" {
			return 100
		}
		return 60
	case "restart":
		return 80
	case "stop":
		return 80
	case "failed":
		return 90
	case "stopped":
		return 70
	case "oom_kill":
		return 100
	case "update":
		return 70
	case "pull":
		return 60
	case "write", "create":
		return 50
	}
	return 0
}

func extractExitCode(e *types.Entry) string {
	// Non-collapsed docker entries store attrs at top level
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal([]byte(e.Metadata), &topLevel); err != nil {
		return ""
	}
	if raw, ok := topLevel["exitCode"]; ok {
		var code string
		if err := json.Unmarshal(raw, &code); err == nil {
			return code
		}
		var numeric int
		if err := json.Unmarshal(raw, &numeric); err == nil {
			return strconv.Itoa(numeric)
		}
	}

	// Collapsed entries store exit code inside raw_events[*].attributes
	rawEventsRaw, ok := topLevel["raw_events"]
	if !ok {
		return ""
	}
	var rawEvents []struct {
		Attributes map[string]string `json:"attributes"`
	}
	if err := json.Unmarshal(rawEventsRaw, &rawEvents); err != nil {
		return ""
	}
	for _, re := range rawEvents {
		if code := re.Attributes["exitCode"]; code != "" {
			return code
		}
	}
	return ""
}

func hasLogSnippet(e *types.Entry) bool {
	var meta struct {
		LogSnippet []string `json:"log_snippet"`
	}
	if err := json.Unmarshal([]byte(e.Metadata), &meta); err != nil {
		return false
	}
	return len(meta.LogSnippet) > 0
}
