package notify

import (
	"context"
	"testing"
	"time"

	"blackbox/server/internal/models"
)

func TestFlushDeferredDigest(t *testing.T) {
	// 08:00 — OUTSIDE a 22:00–07:00 window, so held rows should flush.
	now := time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC)
	d := newTestDispatcher(t, now)

	var sentBodies int
	orig := ntfySender
	ntfySender = func(ctx context.Context, url string, inc models.Incident, event, incURL, note string, test bool) error {
		sentBodies++
		return nil
	}
	defer func() { ntfySender = orig }()

	dest := models.NotificationDest{
		ID: "d1", Type: "ntfy", URL: "https://x/y", Enabled: true,
		QuietHoursEnabled: true, QuietHoursStart: "22:00", QuietHoursEnd: "07:00", QuietHoursMode: "defer",
	}
	d.db.Create(&dest)
	// Two held rows recorded overnight, plus the incidents they reference.
	d.db.Create(&models.Incident{ID: "i1", Title: "DB down", Services: "[]", NodeNames: "[]", Metadata: "{}", Confidence: "confirmed"})
	d.db.Create(&models.Incident{ID: "i2", Title: "Proxy flap", Services: "[]", NodeNames: "[]", Metadata: "{}", Confidence: "suspected"})
	d.logDecision(d.db, "d1", "i1", "incident_opened_confirmed", decisionHeld, "", now.Add(-3*time.Hour))
	d.logDecision(d.db, "d1", "i2", "incident_opened_suspected", decisionHeld, "", now.Add(-2*time.Hour))

	f := NewFlusher(d.db)
	f.now = func() time.Time { return now }
	if err := f.flushAll(context.Background()); err != nil {
		t.Fatalf("flushAll: %v", err)
	}

	if sentBodies != 1 {
		t.Fatalf("expected exactly one digest send, got %d", sentBodies)
	}
	var pending int64
	d.db.Model(&models.NotificationLog{}).
		Where("dest_id = ? AND decision = ? AND flushed_at IS NULL", "d1", decisionHeld).Count(&pending)
	if pending != 0 {
		t.Fatalf("expected held rows flushed, %d still pending", pending)
	}
}

func TestFlushSkipsInsideWindow(t *testing.T) {
	// 03:00 — INSIDE the window, must not flush.
	now := time.Date(2026, 6, 2, 3, 0, 0, 0, time.UTC)
	d := newTestDispatcher(t, now)
	dest := models.NotificationDest{ID: "d1", Type: "ntfy", URL: "https://x/y", Enabled: true,
		QuietHoursEnabled: true, QuietHoursStart: "22:00", QuietHoursEnd: "07:00", QuietHoursMode: "defer"}
	d.db.Create(&dest)
	d.db.Create(&models.Incident{ID: "i1", Title: "x", Services: "[]", NodeNames: "[]", Metadata: "{}"})
	d.logDecision(d.db, "d1", "i1", "incident_opened_confirmed", decisionHeld, "", now.Add(-1*time.Hour))

	f := NewFlusher(d.db)
	f.now = func() time.Time { return now }
	if err := f.flushAll(context.Background()); err != nil {
		t.Fatalf("flushAll: %v", err)
	}
	var pending int64
	d.db.Model(&models.NotificationLog{}).Where("decision = ? AND flushed_at IS NULL", decisionHeld).Count(&pending)
	if pending != 1 {
		t.Fatalf("held row should remain pending inside window, pending=%d", pending)
	}
}
