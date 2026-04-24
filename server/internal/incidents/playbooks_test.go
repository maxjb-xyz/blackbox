package incidents

import (
	"reflect"
	"testing"

	"blackbox/server/internal/models"
)

func TestMatchPlaybooks(t *testing.T) {
	pb := func(id, name, pattern string, priority int, enabled bool) models.Playbook {
		return models.Playbook{ID: id, Name: name, ServicePattern: pattern, Priority: priority, Enabled: enabled}
	}

	cases := []struct {
		name      string
		playbooks []models.Playbook
		services  []string
		wantIDs   []string
	}{
		{
			name:      "no playbooks",
			playbooks: nil,
			services:  []string{"docker:nextcloud"},
			wantIDs:   nil,
		},
		{
			name:      "no services",
			playbooks: []models.Playbook{pb("a", "A", "*", 0, true)},
			services:  nil,
			wantIDs:   nil,
		},
		{
			name: "exact match",
			playbooks: []models.Playbook{
				pb("a", "A", "docker:nextcloud", 0, true),
				pb("b", "B", "docker:other", 0, true),
			},
			services: []string{"docker:nextcloud"},
			wantIDs:  []string{"a"},
		},
		{
			name: "glob prefix",
			playbooks: []models.Playbook{
				pb("a", "A", "qmstart:*", 0, true),
			},
			services: []string{"qmstart:101"},
			wantIDs:  []string{"a"},
		},
		{
			name: "glob suffix",
			playbooks: []models.Playbook{
				pb("a", "A", "*:nextcloud", 0, true),
			},
			services: []string{"docker:nextcloud"},
			wantIDs:  []string{"a"},
		},
		{
			name: "disabled skipped",
			playbooks: []models.Playbook{
				pb("a", "A", "*", 0, false),
				pb("b", "B", "*", 0, true),
			},
			services: []string{"anything"},
			wantIDs:  []string{"b"},
		},
		{
			name: "empty pattern skipped",
			playbooks: []models.Playbook{
				pb("a", "A", "", 0, true),
				pb("b", "B", "anything", 0, true),
			},
			services: []string{"anything"},
			wantIDs:  []string{"b"},
		},
		{
			name: "priority orders results",
			playbooks: []models.Playbook{
				pb("low", "Low", "*", 0, true),
				pb("high", "High", "*", 10, true),
				pb("mid", "Mid", "*", 5, true),
			},
			services: []string{"x"},
			wantIDs:  []string{"high", "mid", "low"},
		},
		{
			name: "same priority sorted by name",
			playbooks: []models.Playbook{
				pb("b", "Bravo", "*", 0, true),
				pb("a", "Alpha", "*", 0, true),
			},
			services: []string{"x"},
			wantIDs:  []string{"a", "b"},
		},
		{
			name: "dedup on multiple service matches",
			playbooks: []models.Playbook{
				pb("a", "A", "*", 0, true),
			},
			services: []string{"svc1", "svc2", "svc3"},
			wantIDs:  []string{"a"},
		},
		{
			name: "invalid pattern does not panic",
			playbooks: []models.Playbook{
				pb("a", "A", "[unclosed", 0, true),
				pb("b", "B", "valid", 0, true),
			},
			services: []string{"valid"},
			wantIDs:  []string{"b"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchPlaybooks(tc.playbooks, tc.services)
			gotIDs := make([]string, len(got))
			for i, p := range got {
				gotIDs[i] = p.ID
			}
			if len(tc.wantIDs) == 0 && len(gotIDs) == 0 {
				return
			}
			if !reflect.DeepEqual(gotIDs, tc.wantIDs) {
				t.Errorf("got %v, want %v", gotIDs, tc.wantIDs)
			}
		})
	}
}
