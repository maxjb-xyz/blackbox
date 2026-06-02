package notify

import (
	"testing"
	"time"
)

func TestDispatcherClockDefaultsAndOverride(t *testing.T) {
	d := NewDispatcher(nil)
	if d.now == nil {
		t.Fatal("default clock must be set")
	}
	fixed := time.Date(2026, 6, 2, 3, 0, 0, 0, time.UTC)
	d.now = func() time.Time { return fixed }
	if !d.now().Equal(fixed) {
		t.Fatal("clock override failed")
	}
}
