package hub_test

import (
	"testing"
	"time"

	"blackbox/server/internal/hub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHub_BroadcastReachesSubscribers(t *testing.T) {
	h := hub.New()

	_, ch1, _, unsub1, err := h.Subscribe("u1", "10.0.0.1")
	require.NoError(t, err)
	_, ch2, _, unsub2, err := h.Subscribe("u2", "10.0.0.2")
	require.NoError(t, err)
	defer unsub1()
	defer unsub2()

	h.Broadcast([]byte(`{"type":"test"}`))

	select {
	case msg := <-ch1:
		assert.Equal(t, `{"type":"test"}`, string(msg))
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch1 did not receive message")
	}

	select {
	case msg := <-ch2:
		assert.Equal(t, `{"type":"test"}`, string(msg))
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch2 did not receive message")
	}
}

func TestHub_UnsubscribedClientReceivesNothing(t *testing.T) {
	h := hub.New()

	_, ch, _, unsub, err := h.Subscribe("u1", "10.0.0.1")
	require.NoError(t, err)
	unsub()

	h.Broadcast([]byte(`{"type":"test"}`))

	select {
	case msg, ok := <-ch:
		if ok {
			t.Fatalf("unsubscribed channel received unexpected message: %s", msg)
		}
		// channel was closed by unsub — that's expected, not a broadcast
	case <-time.After(50 * time.Millisecond):
		// also correct: close signal not yet observed
	}
}

func TestHub_SlowClientDoesNotBlockBroadcast(t *testing.T) {
	h := hub.New()

	// Subscribe with a channel that fills immediately (buffer size 32)
	_, _, _, unsub, err := h.Subscribe("u1", "10.0.0.1")
	require.NoError(t, err)
	defer unsub()

	// Fill that channel by broadcasting 33 messages - should not deadlock
	done := make(chan struct{})
	go func() {
		for i := 0; i < 33; i++ {
			h.Broadcast([]byte(`{}`))
		}
		close(done)
	}()

	select {
	case <-done:
		// correct
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Broadcast blocked on slow client")
	}
}

func TestHub_UnsubscribeIsIdempotent(t *testing.T) {
	h := hub.New()

	_, _, _, unsub, err := h.Subscribe("u1", "10.0.0.1")
	require.NoError(t, err)
	unsub()
	unsub()
}

func TestHub_InvalidateUserDisconnectsMatchingClients(t *testing.T) {
	h := hub.New()

	_, _, disconnect1, unsub1, err := h.Subscribe("u1", "10.0.0.1")
	require.NoError(t, err)
	defer unsub1()

	_, ch2, disconnect2, unsub2, err := h.Subscribe("u2", "10.0.0.2")
	require.NoError(t, err)
	defer unsub2()

	h.InvalidateUser("u1")

	select {
	case reason := <-disconnect1:
		assert.Equal(t, "session invalidated", reason)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("invalidated client did not receive disconnect signal")
	}

	select {
	case <-disconnect2:
		t.Fatal("non-matching client should not be disconnected")
	case <-time.After(50 * time.Millisecond):
	}

	h.Broadcast([]byte(`{"type":"test"}`))

	select {
	case msg := <-ch2:
		assert.Equal(t, `{"type":"test"}`, string(msg))
	case <-time.After(100 * time.Millisecond):
		t.Fatal("remaining client did not receive message")
	}
}

func TestHub_SubscribeEnforcesPerUserLimit(t *testing.T) {
	h := hub.New()

	unsubs := make([]func(), 0, 5)
	for i := 0; i < 5; i++ {
		_, _, _, unsub, err := h.Subscribe("u1", "10.0.0.1")
		require.NoError(t, err)
		unsubs = append(unsubs, unsub)
	}
	defer func() {
		for _, unsub := range unsubs {
			unsub()
		}
	}()

	_, _, _, _, err := h.Subscribe("u1", "10.0.0.2")
	require.ErrorIs(t, err, hub.ErrTooManyUserConnections)
}

func TestHub_SubscribeEnforcesPerIPLimit(t *testing.T) {
	h := hub.New()

	unsubs := make([]func(), 0, 20)
	for i := 0; i < 20; i++ {
		userID := time.Now().Add(time.Duration(i) * time.Nanosecond).Format(time.RFC3339Nano)
		_, _, _, unsub, err := h.Subscribe(userID, "10.0.0.1")
		require.NoError(t, err)
		unsubs = append(unsubs, unsub)
	}
	defer func() {
		for _, unsub := range unsubs {
			unsub()
		}
	}()

	_, _, _, _, err := h.Subscribe("overflow", "10.0.0.1")
	require.ErrorIs(t, err, hub.ErrTooManyIPConnections)
}
