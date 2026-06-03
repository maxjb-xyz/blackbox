package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestNotificationLogRoundTrip(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := gdb.AutoMigrate(&NotificationLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	in := NotificationLog{
		ID:         "log1",
		DestID:     "dest1",
		IncidentID: "inc1",
		Event:      "incident_opened_confirmed",
		Decision:   "sent",
		Note:       "(+2 suppressed in the last hour)",
		CreatedAt:  now,
	}
	if err := gdb.Create(&in).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var out NotificationLog
	if err := gdb.First(&out, "id = ?", "log1").Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if out.Decision != "sent" || out.Note != in.Note || out.FlushedAt != nil {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}
