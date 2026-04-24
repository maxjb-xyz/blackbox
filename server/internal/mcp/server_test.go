package mcp

import (
	"context"
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

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
