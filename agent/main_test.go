package main

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"blackbox/agent/internal/client"
	"blackbox/agent/internal/files"
	"blackbox/agent/internal/systemd"
)

func TestLoadSystemdUnits(t *testing.T) {
	t.Parallel()

	c := client.NewWithHTTPClient("http://blackbox.test", "token", "node-1", &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Fatalf("method = %q, want GET", r.Method)
			}
			if r.URL.Path != "/api/agent/config" {
				t.Fatalf("unexpected path %q", r.URL.Path)
			}
			if got := r.Header.Get("X-Blackbox-Agent-Key"); got != "token" {
				t.Fatalf("X-Blackbox-Agent-Key = %q, want %q", got, "token")
			}
			if got := r.Header.Get("X-Blackbox-Node-Name"); got != "node-1" {
				t.Fatalf("X-Blackbox-Node-Name = %q, want %q", got, "node-1")
			}
			if got := r.Header.Get("X-Blackbox-Agent-Capabilities"); got != "docker,systemd" {
				t.Fatalf("X-Blackbox-Agent-Capabilities = %q, want %q", got, "docker,systemd")
			}
			return jsonResponse(http.StatusOK, `{"systemd_units":["nginx.service","postgres.service"]}`), nil
		}),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	units, err := loadSystemdUnits(ctx, c, []string{"docker", "systemd"})
	if err != nil {
		t.Fatalf("loadSystemdUnits() error = %v", err)
	}

	want := []string{"nginx.service", "postgres.service"}
	if !reflect.DeepEqual(units, want) {
		t.Fatalf("loadSystemdUnits() = %v, want %v", units, want)
	}
}

func TestRefreshSystemdSettingsKeepsExistingUnitsOnFetchFailure(t *testing.T) {
	t.Parallel()

	c := client.NewWithHTTPClient("http://blackbox.test", "token", "node-1", &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %q, want GET", r.Method)
			}
			if r.URL.Path != "/api/agent/config" {
				t.Errorf("path = %q, want /api/agent/config", r.URL.Path)
			}
			if got := r.Header.Get("X-Blackbox-Agent-Key"); got != "token" {
				t.Errorf("X-Blackbox-Agent-Key = %q, want %q", got, "token")
			}
			if got := r.Header.Get("X-Blackbox-Node-Name"); got != "node-1" {
				t.Errorf("X-Blackbox-Node-Name = %q, want %q", got, "node-1")
			}
			if got := r.Header.Get("X-Blackbox-Agent-Capabilities"); got != "" {
				t.Errorf("X-Blackbox-Agent-Capabilities = %q, want empty", got)
			}
			return jsonResponse(http.StatusBadGateway, `boom`), nil
		}),
	})
	settings := systemd.NewSettings([]string{"nginx.service"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Unbuffered: send blocks until goroutine receives, guaranteeing the case
	// body executes to completion before we cancel and read settings.
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		defer close(done)
		refreshSystemdSettingsWithTicker(ctx, c, []string{}, settings, ticks)
	}()

	ticks <- time.Now()
	cancel()
	<-done

	want := []string{"nginx.service"}
	if got := settings.Units(); !reflect.DeepEqual(got, want) {
		t.Fatalf("settings.Units() = %v, want %v", got, want)
	}
}

func TestRefreshSystemdSettingsUpdatesUnitsOnSuccess(t *testing.T) {
	t.Parallel()

	c := client.NewWithHTTPClient("http://blackbox.test", "token", "node-1", &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %q, want GET", r.Method)
			}
			if r.URL.Path != "/api/agent/config" {
				t.Errorf("path = %q, want /api/agent/config", r.URL.Path)
			}
			if got := r.Header.Get("X-Blackbox-Agent-Key"); got != "token" {
				t.Errorf("X-Blackbox-Agent-Key = %q, want %q", got, "token")
			}
			if got := r.Header.Get("X-Blackbox-Node-Name"); got != "node-1" {
				t.Errorf("X-Blackbox-Node-Name = %q, want %q", got, "node-1")
			}
			if got := r.Header.Get("X-Blackbox-Agent-Capabilities"); got != "docker,systemd" {
				t.Errorf("X-Blackbox-Agent-Capabilities = %q, want %q", got, "docker,systemd")
			}
			return jsonResponse(http.StatusOK, `{"systemd_units":["redis.service"]}`), nil
		}),
	})
	settings := systemd.NewSettings([]string{"nginx.service"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Unbuffered: send blocks until goroutine receives, guaranteeing the case
	// body executes to completion before we cancel and read settings.
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		defer close(done)
		refreshSystemdSettingsWithTicker(ctx, c, []string{"docker", "systemd"}, settings, ticks)
	}()

	ticks <- time.Now()
	cancel()
	<-done

	want := []string{"redis.service"}
	if got := settings.Units(); !reflect.DeepEqual(got, want) {
		t.Fatalf("settings.Units() = %v, want %v", got, want)
	}
}

func TestRefreshFileWatcherSettingsKeepsExistingSettingsOnFetchFailure(t *testing.T) {
	t.Parallel()

	c := client.NewWithHTTPClient("http://blackbox.test", "token", "node-1", &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %q, want GET", r.Method)
			}
			if r.URL.Path != "/api/agent/config" {
				t.Errorf("path = %q, want /api/agent/config", r.URL.Path)
			}
			return jsonResponse(http.StatusBadGateway, `boom`), nil
		}),
	})
	settings := files.NewSettings(false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		defer close(done)
		refreshFileWatcherSettingsWithTicker(ctx, c, []string{"docker", "filewatcher"}, settings, ticks)
	}()

	ticks <- time.Now()
	cancel()
	<-done

	if !settings.Enabled() {
		t.Fatalf("settings.Enabled() = false, want true")
	}
	if settings.RedactSecrets() {
		t.Fatalf("settings.RedactSecrets() = true, want false")
	}
}

func TestBuildCapabilities_OnlyAdvertisesSystemdWhenSupportedAndEnabled(t *testing.T) {
	t.Setenv("WATCH_SYSTEMD", "true")

	caps := buildCapabilities([]string{"/etc"})

	if !slices.Contains(caps, "docker") {
		t.Fatalf("buildCapabilities() = %v, missing docker", caps)
	}
	if !slices.Contains(caps, "filewatcher") {
		t.Fatalf("buildCapabilities() = %v, missing filewatcher", caps)
	}

	hasSystemd := slices.Contains(caps, "systemd")
	if systemd.Supported() && !hasSystemd {
		t.Fatalf("buildCapabilities() = %v, missing systemd when supported", caps)
	}
	if !systemd.Supported() && hasSystemd {
		t.Fatalf("buildCapabilities() = %v, unexpectedly includes systemd when unsupported", caps)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
