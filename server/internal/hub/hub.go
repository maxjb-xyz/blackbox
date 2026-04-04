package hub

import (
	"errors"
	"sync"

	"github.com/oklog/ulid/v2"
)

const clientBufferSize = 32

const (
	maxConnectionsPerUser = 5
	maxConnectionsPerIP   = 20
)

var (
	ErrTooManyUserConnections = errors.New("too many websocket connections for user")
	ErrTooManyIPConnections   = errors.New("too many websocket connections from ip")
)

type client struct {
	userID       string
	remoteAddr   string
	msgCh        chan []byte
	disconnectCh chan string
	once         sync.Once
}

func (c *client) requestDisconnect(reason string) {
	select {
	case c.disconnectCh <- reason:
	default:
	}
}

func (c *client) close() {
	c.once.Do(func() {
		close(c.msgCh)
		close(c.disconnectCh)
	})
}

// Hub broadcasts byte slices to all subscribed channels.
// Safe for concurrent use.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*client
}

func New() *Hub {
	return &Hub{clients: make(map[string]*client)}
}

// Subscribe registers a new client and returns its ID, receive channel,
// disconnect signal, and an unsubscribe function that must be called when the
// client disconnects.
func (h *Hub) Subscribe(userID, remoteAddr string) (id string, ch <-chan []byte, disconnect <-chan string, unsub func(), err error) {
	id = ulid.Make().String()
	c := &client{
		userID:       userID,
		remoteAddr:   remoteAddr,
		msgCh:        make(chan []byte, clientBufferSize),
		disconnectCh: make(chan string, 1),
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if userID != "" && h.countClientsByUserLocked(userID) >= maxConnectionsPerUser {
		return "", nil, nil, nil, ErrTooManyUserConnections
	}
	if remoteAddr != "" && h.countClientsByIPLocked(remoteAddr) >= maxConnectionsPerIP {
		return "", nil, nil, nil, ErrTooManyIPConnections
	}

	h.clients[id] = c

	return id, c.msgCh, c.disconnectCh, func() {
		h.removeClient(id)
	}, nil
}

// Broadcast sends msg to all subscribed clients. Slow clients are skipped
// (non-blocking send) to prevent any single client from stalling broadcasts.
func (h *Hub) Broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		select {
		case c.msgCh <- msg:
		default:
		}
	}
}

// InvalidateUser disconnects all active connections for a user.
func (h *Hub) InvalidateUser(userID string) {
	if userID == "" {
		return
	}

	clients := h.removeClients(func(c *client) bool {
		return c.userID == userID
	})
	for _, c := range clients {
		c.requestDisconnect("session invalidated")
	}
}

func (h *Hub) removeClient(id string) {
	h.mu.Lock()
	c, ok := h.clients[id]
	if ok {
		delete(h.clients, id)
	}
	h.mu.Unlock()
	if ok {
		c.close()
	}
}

func (h *Hub) removeClients(match func(*client) bool) []*client {
	h.mu.Lock()
	defer h.mu.Unlock()

	removed := make([]*client, 0)
	for id, c := range h.clients {
		if !match(c) {
			continue
		}
		delete(h.clients, id)
		removed = append(removed, c)
	}
	return removed
}

func (h *Hub) countClientsByUserLocked(userID string) int {
	count := 0
	for _, c := range h.clients {
		if c.userID == userID {
			count++
		}
	}
	return count
}

func (h *Hub) countClientsByIPLocked(remoteAddr string) int {
	count := 0
	for _, c := range h.clients {
		if c.remoteAddr == remoteAddr {
			count++
		}
	}
	return count
}
