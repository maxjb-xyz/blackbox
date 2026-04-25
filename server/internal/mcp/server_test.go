package mcp

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestMCPManagerStartStop(t *testing.T) {
	port := freePort(t)
	manager := NewMCPManager(nil)

	if err := manager.ApplySettings(true, port, "secret"); err != nil {
		t.Fatalf("ApplySettings start: %v", err)
	}
	if !manager.IsRunning() {
		t.Fatal("expected manager to be running")
	}
	if err := manager.ApplySettings(false, port, "secret"); err != nil {
		t.Fatalf("ApplySettings stop: %v", err)
	}
	if manager.IsRunning() {
		t.Fatal("expected manager to be stopped")
	}
}

func TestMCPManagerRestart(t *testing.T) {
	manager := NewMCPManager(nil)
	port := freePort(t)

	if err := manager.ApplySettings(true, port, "secret"); err != nil {
		t.Fatalf("start: %v", err)
	}
	nextPort := freePort(t)
	if err := manager.ApplySettings(true, nextPort, "new-secret"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !manager.IsRunning() {
		t.Fatal("expected manager to be running after restart")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestMCPManagerSamePortRestart(t *testing.T) {
	port := freePort(t)
	mgr := NewMCPManager(nil)

	// Start on port
	if err := mgr.ApplySettings(true, port, "token1"); err != nil {
		t.Fatalf("initial start: %v", err)
	}
	if !mgr.IsRunning() {
		t.Fatal("expected running after start")
	}

	// Same-port restart with new token (simulates token rotation)
	if err := mgr.ApplySettings(true, port, "token2"); err != nil {
		t.Fatalf("same-port restart: %v", err)
	}
	if !mgr.IsRunning() {
		t.Fatal("expected still running after same-port restart")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := mgr.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestMCPManagerRepeatedSamePortRestart(t *testing.T) {
	port := freePort(t)
	mgr := NewMCPManager(nil)

	if err := mgr.ApplySettings(true, port, "token0"); err != nil {
		t.Fatalf("initial start: %v", err)
	}

	for i := 1; i <= 20; i++ {
		if err := mgr.ApplySettings(true, port, fmt.Sprintf("token%d", i)); err != nil {
			t.Fatalf("same-port restart %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := mgr.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}
