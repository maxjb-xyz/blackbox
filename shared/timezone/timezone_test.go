package timezone

import (
	"testing"
	"time"
)

func TestConfigureLocalUsesTZEnv(t *testing.T) {
	t.Setenv("TZ", "America/New_York")

	original := time.Local
	t.Cleanup(func() {
		time.Local = original
	})

	tz, err := ConfigureLocal()
	if err != nil {
		t.Fatalf("ConfigureLocal() error = %v", err)
	}
	if tz != "America/New_York" {
		t.Fatalf("ConfigureLocal() tz = %q, want %q", tz, "America/New_York")
	}
	if time.Local.String() != "America/New_York" {
		t.Fatalf("time.Local = %q, want %q", time.Local.String(), "America/New_York")
	}
}

func TestConfigureLocalRejectsInvalidTZ(t *testing.T) {
	t.Setenv("TZ", "Mars/Olympus")

	original := time.Local
	t.Cleanup(func() {
		time.Local = original
	})

	tz, err := ConfigureLocal()
	if err == nil {
		t.Fatal("ConfigureLocal() error = nil, want non-nil")
	}
	if tz != "Mars/Olympus" {
		t.Fatalf("ConfigureLocal() tz = %q, want %q", tz, "Mars/Olympus")
	}
	if time.Local != original {
		t.Fatal("time.Local changed on invalid TZ")
	}
}
