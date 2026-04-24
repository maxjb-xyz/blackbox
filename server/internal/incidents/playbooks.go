package incidents

import (
	"path"
	"sort"

	"blackbox/server/internal/models"
)

// MatchPlaybooks returns the playbooks whose ServicePattern matches any
// of the given services. Only enabled playbooks are considered. Results
// are deduplicated by ID and sorted by Priority descending, then Name
// ascending for a stable order. The return value is always non-nil so
// callers can safely serialize it as a JSON array.
func MatchPlaybooks(playbooks []models.Playbook, services []string) []models.Playbook {
	matched := make([]models.Playbook, 0)
	if len(playbooks) == 0 || len(services) == 0 {
		return matched
	}

	seen := make(map[string]struct{}, len(playbooks))

	for _, pb := range playbooks {
		if !pb.Enabled || pb.ServicePattern == "" {
			continue
		}
		if _, already := seen[pb.ID]; already {
			continue
		}
		for _, svc := range services {
			ok, err := path.Match(pb.ServicePattern, svc)
			if err == nil && ok {
				matched = append(matched, pb)
				seen[pb.ID] = struct{}{}
				break
			}
		}
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].Priority != matched[j].Priority {
			return matched[i].Priority > matched[j].Priority
		}
		return matched[i].Name < matched[j].Name
	})
	return matched
}
