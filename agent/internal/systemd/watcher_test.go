package systemd

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestSettings_GetSet(t *testing.T) {
	s := NewSettings([]string{"nginx.service", "postgres.service"})
	units := s.Units()
	if len(units) != 2 {
		t.Fatalf("expected 2 units, got %d", len(units))
	}
	if units[0] != "nginx.service" || units[1] != "postgres.service" {
		t.Fatalf("unexpected units: %v", units)
	}
}

func TestSettings_SetUnits_ReplacesList(t *testing.T) {
	s := NewSettings([]string{"nginx.service"})
	s.SetUnits([]string{"redis.service", "postgres.service"})
	units := s.Units()
	if len(units) != 2 || units[0] != "redis.service" {
		t.Fatalf("expected updated list, got %v", units)
	}
}

func TestSettings_ConcurrentAccess(t *testing.T) {
	s := NewSettings([]string{"nginx.service"})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.SetUnits([]string{"nginx.service", "redis.service"})
		}()
		go func() {
			defer wg.Done()
			_ = s.Units()
		}()
	}
	wg.Wait()
}

func TestSettings_HasUnit(t *testing.T) {
	s := NewSettings([]string{"nginx.service", "redis.service"})
	if !s.HasUnit("redis.service") {
		t.Fatal("expected HasUnit to report configured unit")
	}
	if s.HasUnit("postgres.service") {
		t.Fatal("did not expect HasUnit to match missing unit")
	}
}

func TestMapTransition(t *testing.T) {
	cases := []struct {
		prev string
		curr string
		want string
	}{
		{"active", "failed", "failed"},
		{"failed", "activating", "restart"},
		{"activating", "active", "started"},
		{"active", "deactivating", "stopped"},
		{"deactivating", "inactive", "stopped"},
		{"inactive", "active", "started"},
		{"active", "active", ""},
		{"", "active", ""},
	}
	for _, tc := range cases {
		got := mapTransition(tc.prev, tc.curr)
		if got != tc.want {
			t.Errorf("mapTransition(%q, %q) = %q, want %q", tc.prev, tc.curr, got, tc.want)
		}
	}
}

func TestTrackedUnit_PrefersManagerFields(t *testing.T) {
	fields := map[string]string{
		"_SYSTEMD_UNIT":       "systemd.service",
		"UNIT":                "nginx.service",
		"OBJECT_SYSTEMD_UNIT": "ignored.service",
	}

	got := trackedUnit(fields, []string{"nginx.service"})
	if got != "nginx.service" {
		t.Fatalf("trackedUnit() = %q, want nginx.service", got)
	}
}

func TestClassifyLifecycleEvent(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]string
		units  []string
		unit   string
		event  string
	}{
		{
			name: "started from manager unit message",
			fields: map[string]string{
				"UNIT":    "nginx.service",
				"_PID":    "1",
				"MESSAGE": "Started A high performance web server.",
			},
			units: []string{"nginx.service"},
			unit:  "nginx.service",
			event: "started",
		},
		{
			name: "stopped from manager unit message",
			fields: map[string]string{
				"UNIT":    "nginx.service",
				"_PID":    "1",
				"MESSAGE": "Stopped A high performance web server.",
			},
			units: []string{"nginx.service"},
			unit:  "nginx.service",
			event: "stopped",
		},
		{
			name: "restart from service manager log",
			fields: map[string]string{
				"UNIT":    "nginx.service",
				"_PID":    "1",
				"MESSAGE": "nginx.service: Scheduled restart job, restart counter is at 3.",
			},
			units: []string{"nginx.service"},
			unit:  "nginx.service",
			event: "restart",
		},
		{
			name: "failed from result message",
			fields: map[string]string{
				"UNIT":    "nginx.service",
				"_PID":    "1",
				"MESSAGE": "nginx.service: Failed with result 'exit-code'.",
			},
			units: []string{"nginx.service"},
			unit:  "nginx.service",
			event: "failed",
		},
		{
			name: "ignores unrelated unit process logs",
			fields: map[string]string{
				"_SYSTEMD_UNIT": "nginx.service",
				"MESSAGE":       "worker process started",
			},
			units: []string{"nginx.service"},
		},
		{
			name: "ignores service logs that happen to start with Started",
			fields: map[string]string{
				"_SYSTEMD_UNIT": "nginx.service",
				"MESSAGE":       "Started cache warmup task",
			},
			units: []string{"nginx.service"},
		},
		{
			name: "ignores unwatched unit",
			fields: map[string]string{
				"UNIT":    "postgres.service",
				"_PID":    "1",
				"MESSAGE": "Started PostgreSQL server.",
			},
			units: []string{"nginx.service"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unit, event := classifyLifecycleEvent(tc.fields, tc.units)
			if unit != tc.unit || event != tc.event {
				t.Fatalf("classifyLifecycleEvent() = (%q, %q), want (%q, %q)", unit, event, tc.unit, tc.event)
			}
		})
	}
}

func TestRebuildJournalMatches(t *testing.T) {
	m := &fakeJournalMatcher{}

	err := rebuildJournalMatches(m, []string{"nginx.service"})
	if err != nil {
		t.Fatalf("rebuildJournalMatches() error = %v", err)
	}

	want := []string{
		"flush",
		"match:_SYSTEMD_UNIT=nginx.service",
		"or",
		"match:UNIT=nginx.service",
		"and",
		"match:_PID=1",
		"or",
		"match:OBJECT_SYSTEMD_UNIT=nginx.service",
		"and",
		"match:_UID=0",
		"or",
		"match:SYSLOG_FACILITY=0",
	}
	if !reflect.DeepEqual(m.ops, want) {
		t.Fatalf("rebuildJournalMatches() ops = %#v, want %#v", m.ops, want)
	}
}

func TestApplyMatchGroups_PropagatesErrors(t *testing.T) {
	m := &fakeJournalMatcher{addConjunctionErr: errors.New("boom")}
	err := applyMatchGroups(m, [][]string{{"UNIT=nginx.service", "_PID=1"}})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("applyMatchGroups() error = %v, want boom", err)
	}
}

type fakeJournalMatcher struct {
	ops               []string
	addMatchErr       error
	addDisjunctionErr error
	addConjunctionErr error
}

func (f *fakeJournalMatcher) FlushMatches() {
	f.ops = append(f.ops, "flush")
}

func (f *fakeJournalMatcher) AddMatch(match string) error {
	f.ops = append(f.ops, "match:"+match)
	return f.addMatchErr
}

func (f *fakeJournalMatcher) AddDisjunction() error {
	f.ops = append(f.ops, "or")
	return f.addDisjunctionErr
}

func (f *fakeJournalMatcher) AddConjunction() error {
	f.ops = append(f.ops, "and")
	return f.addConjunctionErr
}
