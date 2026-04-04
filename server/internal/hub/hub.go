package hub

import (
	"sync"

	"github.com/oklog/ulid/v2"
)

const clientBufferSize = 32

// Hub broadcasts byte slices to all subscribed channels.
// Safe for concurrent use.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]chan []byte
}

func New() *Hub {
	return &Hub{clients: make(map[string]chan []byte)}
}

// Subscribe registers a new client and returns its ID, receive channel, and
// an unsubscribe function that must be called when the client disconnects.
func (h *Hub) Subscribe() (id string, ch <-chan []byte, unsub func()) {
	id = ulid.Make().String()
	c := make(chan []byte, clientBufferSize)

	h.mu.Lock()
	h.clients[id] = c
	h.mu.Unlock()

	return id, c, func() {
		h.mu.Lock()
		delete(h.clients, id)
		h.mu.Unlock()
	}
}

// Broadcast sends msg to all subscribed clients. Slow clients are skipped
// (non-blocking send) to prevent any single client from stalling broadcasts.
func (h *Hub) Broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		select {
		case c <- msg:
		default:
		}
	}
}
