package systemd

import (
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
