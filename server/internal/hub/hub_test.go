package hub_test

import (
	"testing"
	"time"

	"blackbox/server/internal/hub"
	"github.com/stretchr/testify/assert"
)

func TestHub_BroadcastReachesSubscribers(t *testing.T) {
	h := hub.New()

	_, ch1, unsub1 := h.Subscribe()
	_, ch2, unsub2 := h.Subscribe()
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

	_, ch, unsub := h.Subscribe()
	unsub()

	h.Broadcast([]byte(`{"type":"test"}`))

	select {
	case <-ch:
		t.Fatal("unsubscribed channel should not receive")
	case <-time.After(50 * time.Millisecond):
		// correct: nothing received
	}
}

func TestHub_SlowClientDoesNotBlockBroadcast(t *testing.T) {
	h := hub.New()

	// Subscribe with a channel that fills immediately (buffer size 32)
	_, _, unsub := h.Subscribe()
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
