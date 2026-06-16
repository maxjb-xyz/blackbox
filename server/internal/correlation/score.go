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
func ScoreCauses(db *gorm.DB, services []string, node string, at time.Time, triggerComposeService string) ([]CauseCandidate, error) {
	if len(services) == 0 {
		return []CauseCandidate{}, nil
	}
	windowStart := at.Add(-maxLookbackWindow)

	// Images the trigger services actually run, learned from their container
	// (non pull/delete) entries. Pull/delete events correlate by image, not by
	// the synthetic service token the agent assigns to shared-image pulls.
	imagesUsed, err := imagesForServices(db, services, node)
	if err != nil {
		return nil, err
	}

	query := db.Where("timestamp BETWEEN ? AND ?", windowStart, at).
		Where("NOT (source = ? AND event IN ?)", "webhook", []string{"down", "up"})

	pullSources := []string{"pull", "delete"}
	if len(imagesUsed) > 0 {
		query = query.Where(
			db.Where("service IN ? AND NOT (source = ? AND event IN ?)", services, "docker", pullSources).
				Or("source = ? AND event IN ? AND image IN ?", "docker", pullSources, imagesUsed),
		)
	} else {
		query = query.Where("service IN ? AND NOT (source = ? AND event IN ?)", services, "docker", pullSources)
	}

	if node != "" {
		query = query.Where("node_name = ? OR node_name = ?", node, "")
	}

	if triggerComposeService != "" {
		query = query.Where(
			"NOT (source = ? AND compose_service != ? AND compose_service != ?)",
			"docker", "", triggerComposeService,
		)
	}

	var candidates []types.Entry
	if err := query.Order("timestamp DESC").Find(&candidates).Error; err != nil {
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
		elapsedSeconds := at.Sub(e.Timestamp).Seconds()
		windowSeconds := window.Seconds()
		decayFactor := 1.0 - (elapsedSeconds/windowSeconds)*0.4
		score := int(float64(base+bonus) * decayFactor)
		if score < MinCauseScore {
			continue
		}
		elapsed := int(at.Sub(e.Timestamp).Seconds())
		results = append(results, CauseCandidate{
			Entry:  e,
			Score:  score,
			Reason: fmt.Sprintf("%s %s %ds before trigger (base=%d bonus=%d decay=%.2f score=%d)", e.Source, e.Event, elapsed, base, bonus, decayFactor, score),
		})
	}

	sort.Slice(results, func(i, j int) bool { return causeCandidateLess(results[i], results[j]) })
	return results, nil
}

// imagesForServices returns the distinct normalized images that the given
// services run, derived from their container (non pull/delete) docker entries.
// Scoped to node (plus node-less entries) when node is non-empty.
func imagesForServices(db *gorm.DB, services []string, node string) ([]string, error) {
	q := db.Model(&types.Entry{}).
		Where("service IN ? AND image <> ? AND NOT (source = ? AND event IN ?)",
			services, "", "docker", []string{"pull", "delete"})
	if node != "" {
		q = q.Where("node_name = ? OR node_name = ?", node, "")
	}
	var images []string
	if err := q.Distinct().Pluck("image", &images).Error; err != nil {
		return nil, err
	}
	return images, nil
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
