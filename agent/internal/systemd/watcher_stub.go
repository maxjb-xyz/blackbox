//go:build !linux

package systemd

import (
	"context"

	"blackbox/shared/types"
)

// watch is a no-op stub on non-Linux platforms. The systemd watcher requires Linux.
func watch(ctx context.Context, _ string, _ *Settings, _ chan<- types.Entry) error {
	<-ctx.Done()
	return nil
}
