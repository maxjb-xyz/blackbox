package notify

import (
	"testing"
	"time"

	"blackbox/server/internal/models"
)

// newTestDispatcher builds a Dispatcher over a fully-migrated in-memory DB
// (via newTestDB, which uses a shared-cache DSN with a single connection so
// goroutine sends see the same schema) and pins its clock to `now`.
func newTestDispatcher(t *testing.T, now time.Time) *Dispatcher {
	t.Helper()
	d := NewDispatcher(newTestDB(t))
	d.now = func() time.Time { return now }
	return d
}

func mustLog(t *testing.T, d *Dispatcher, destID, decision string, at time.Time) {
	t.Helper()
	if err := d.logDecision(d.db, destID, "inc", "evt", decision, "", at); err != nil {
		t.Fatalf("logDecision: %v", err)
	}
}

func TestSentCountInWindowAndSuppressed(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	d := newTestDispatcher(t, base)

	// 2 sends 30m and 10m ago, 1 send 2h ago (outside the hour window).
	mustLog(t, d, "dest1", decisionSent, base.Add(-30*time.Minute))
	mustLog(t, d, "dest1", decisionSent, base.Add(-10*time.Minute))
	mustLog(t, d, "dest1", decisionSent, base.Add(-2*time.Hour))
	// 3 rate drops after the most recent sent.
	mustLog(t, d, "dest1", decisionDroppedRate, base.Add(-5*time.Minute))
	mustLog(t, d, "dest1", decisionDroppedRate, base.Add(-4*time.Minute))
	mustLog(t, d, "dest1", decisionDroppedRate, base.Add(-3*time.Minute))

	got, err := d.sentCountSince(d.db, "dest1", base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("sentCountSince: %v", err)
	}
	if got != 2 {
		t.Fatalf("sentCountSince = %d, want 2", got)
	}

	sup, err := d.droppedRateSinceLastSent(d.db, "dest1")
	if err != nil {
		t.Fatalf("droppedRateSinceLastSent: %v", err)
	}
	if sup != 3 {
		t.Fatalf("droppedRateSinceLastSent = %d, want 3", sup)
	}
}

func TestDecideMatrix(t *testing.T) {
	// 03:00 UTC — inside a 22:00-07:00 quiet window.
	base := time.Date(2026, 6, 2, 3, 0, 0, 0, time.UTC)

	t.Run("quiet drop", func(t *testing.T) {
		d := newTestDispatcher(t, base)
		dest := models.NotificationDest{ID: "x", QuietHoursEnabled: true, QuietHoursStart: "22:00", QuietHoursEnd: "07:00", QuietHoursMode: "drop"}
		dec, note, err := d.decide(d.db, dest)
		if err != nil || dec != decisionDroppedQuiet || note != "" {
			t.Fatalf("got (%q,%q,%v)", dec, note, err)
		}
	})

	t.Run("quiet defer", func(t *testing.T) {
		d := newTestDispatcher(t, base)
		dest := models.NotificationDest{ID: "x", QuietHoursEnabled: true, QuietHoursStart: "22:00", QuietHoursEnd: "07:00", QuietHoursMode: "defer"}
		dec, _, _ := d.decide(d.db, dest)
		if dec != decisionHeld {
			t.Fatalf("got %q want held", dec)
		}
	})

	t.Run("rate limited", func(t *testing.T) {
		day := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
		d := newTestDispatcher(t, day)
		dest := models.NotificationDest{ID: "x", RateLimitEnabled: true, RateLimitCount: 2, RateLimitUnit: "hour"}
		mustLog(t, d, "x", decisionSent, day.Add(-10*time.Minute))
		mustLog(t, d, "x", decisionSent, day.Add(-5*time.Minute))
		dec, _, _ := d.decide(d.db, dest)
		if dec != decisionDroppedRate {
			t.Fatalf("got %q want dropped_rate", dec)
		}
	})

	t.Run("sent with suppressed note", func(t *testing.T) {
		day := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
		d := newTestDispatcher(t, day)
		dest := models.NotificationDest{ID: "x", RateLimitEnabled: true, RateLimitCount: 5, RateLimitUnit: "hour"}
		mustLog(t, d, "x", decisionSent, day.Add(-30*time.Minute))
		mustLog(t, d, "x", decisionDroppedRate, day.Add(-2*time.Minute))
		mustLog(t, d, "x", decisionDroppedRate, day.Add(-1*time.Minute))
		dec, note, _ := d.decide(d.db, dest)
		if dec != decisionSent {
			t.Fatalf("got %q want sent", dec)
		}
		if note != "(+2 suppressed in the last hour)" {
			t.Fatalf("note = %q", note)
		}
	})

	t.Run("no policy sends clean", func(t *testing.T) {
		d := newTestDispatcher(t, base)
		dest := models.NotificationDest{ID: "x"}
		dec, note, _ := d.decide(d.db, dest)
		if dec != decisionSent || note != "" {
			t.Fatalf("got (%q,%q)", dec, note)
		}
	})
}
