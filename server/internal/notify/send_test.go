package notify

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"blackbox/server/internal/models"
)

func TestSendRecordsDecisions(t *testing.T) {
	// 03:00 inside a 22:00-07:00 drop window -> dropped_quiet, no HTTP send.
	base := time.Date(2026, 6, 2, 3, 0, 0, 0, time.UTC)
	d := newTestDispatcher(t, base)

	var mu sync.Mutex
	sent := 0
	orig := ntfySender
	ntfySender = func(ctx context.Context, url string, inc models.Incident, event, incURL, note string, test bool) error {
		mu.Lock()
		sent++
		mu.Unlock()
		return nil
	}
	defer func() { ntfySender = orig }()

	dest := models.NotificationDest{
		ID: "d1", Type: "ntfy", URL: "https://x/y", Enabled: true,
		Events:            `["incident_opened_confirmed"]`,
		QuietHoursEnabled: true, QuietHoursStart: "22:00", QuietHoursEnd: "07:00", QuietHoursMode: "drop",
	}
	if err := d.db.Create(&dest).Error; err != nil {
		t.Fatalf("seed dest: %v", err)
	}

	d.Send(context.Background(), EventIncidentOpenedConfirmed, models.Incident{ID: "inc1", Metadata: "{}"})

	// Send fans out in goroutines; wait briefly for the decision row.
	waitForLogRows(t, d, "d1", 1)

	var row models.NotificationLog
	if err := d.db.First(&row, "dest_id = ?", "d1").Error; err != nil {
		t.Fatalf("read log: %v", err)
	}
	if row.Decision != decisionDroppedQuiet {
		t.Fatalf("decision = %q want dropped_quiet", row.Decision)
	}
	mu.Lock()
	defer mu.Unlock()
	if sent != 0 {
		t.Fatalf("expected no HTTP send during quiet drop, got %d", sent)
	}
}

func TestRateLimitHardCapUnderConcurrency(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	d := newTestDispatcher(t, base)

	var sent int32
	orig := ntfySender
	ntfySender = func(ctx context.Context, url string, inc models.Incident, event, incURL, note string, test bool) error {
		atomic.AddInt32(&sent, 1)
		time.Sleep(5 * time.Millisecond) // widen the race window
		return nil
	}
	defer func() { ntfySender = orig }()

	dest := models.NotificationDest{
		ID: "d1", Type: "ntfy", URL: "https://x/y", Enabled: true,
		Events:           `["incident_opened_confirmed"]`,
		RateLimitEnabled: true, RateLimitCount: 1, RateLimitUnit: "hour",
	}
	if err := d.db.Create(&dest).Error; err != nil {
		t.Fatalf("seed dest: %v", err)
	}

	const n = 6
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.evaluateAndSend(dest, models.Incident{ID: "inc", Metadata: "{}"}, EventIncidentOpenedConfirmed, "")
		}()
	}
	wg.Wait()

	var sentRows, droppedRows int64
	d.db.Model(&models.NotificationLog{}).Where("dest_id = ? AND decision = ?", "d1", decisionSent).Count(&sentRows)
	d.db.Model(&models.NotificationLog{}).Where("dest_id = ? AND decision = ?", "d1", decisionDroppedRate).Count(&droppedRows)

	if sentRows != 1 {
		t.Fatalf("hard cap breached: %d sent rows, want 1", sentRows)
	}
	if droppedRows != int64(n-1) {
		t.Fatalf("expected %d dropped_rate rows, got %d", n-1, droppedRows)
	}
	if got := atomic.LoadInt32(&sent); got != 1 {
		t.Fatalf("expected 1 actual delivery, got %d", got)
	}
}

func waitForLogRows(t *testing.T, d *Dispatcher, destID string, want int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int64
		d.db.Model(&models.NotificationLog{}).Where("dest_id = ?", destID).Count(&n)
		if n >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d log rows", want)
}
