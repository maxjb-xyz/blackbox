package handlers

import (
	"testing"
	"time"

	"blackbox/shared/types"
	"github.com/stretchr/testify/require"
)

func TestDispatchToIncidentChannelWithShutdown_ReleasesPendingSend(t *testing.T) {
	shutdown := make(chan struct{})
	ch := make(chan types.Entry)

	require.Len(t, incidentDispatchSem, 0)

	dispatchToIncidentChannelWithShutdown(ch, shutdown, types.Entry{
		ID:      "01TESTENTRY000000",
		Service: "radarr",
	})

	require.Eventually(t, func() bool {
		return len(incidentDispatchSem) == 1
	}, time.Second, 10*time.Millisecond)

	close(shutdown)

	require.Eventually(t, func() bool {
		return len(incidentDispatchSem) == 0
	}, time.Second, 10*time.Millisecond)
}
