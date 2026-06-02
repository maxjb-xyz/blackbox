package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestNotificationDestPolicyColumns(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := gdb.AutoMigrate(&NotificationDest{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	in := NotificationDest{
		ID: "d1", Name: "n", Type: "ntfy", URL: "https://x/y", Events: "[]", Enabled: true,
		QuietHoursEnabled: true, QuietHoursStart: "22:00", QuietHoursEnd: "07:00", QuietHoursMode: "defer",
		RateLimitEnabled: true, RateLimitCount: 5, RateLimitUnit: "hour",
	}
	if err := gdb.Create(&in).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var out NotificationDest
	if err := gdb.First(&out, "id = ?", "d1").Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if !out.QuietHoursEnabled || out.QuietHoursStart != "22:00" || out.QuietHoursMode != "defer" {
		t.Fatalf("quiet hours not persisted: %+v", out)
	}
	if !out.RateLimitEnabled || out.RateLimitCount != 5 || out.RateLimitUnit != "hour" {
		t.Fatalf("rate limit not persisted: %+v", out)
	}
}
