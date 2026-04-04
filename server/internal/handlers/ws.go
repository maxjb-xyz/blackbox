package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/hub"
	"github.com/coder/websocket"
)

const (
	websocketWriteTimeout = 10 * time.Second
	websocketPingInterval = 30 * time.Second
)

func WebSocketHandler(h *hub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			// Same-origin only; browser enforces this, but be explicit
			InsecureSkipVerify: false,
		})
		if err != nil {
			log.Printf("ws: accept error: %v", err)
			return
		}
		closeAttempted := false
		defer func() {
			if closeAttempted {
				return
			}
			if err := conn.CloseNow(); err != nil {
				if isExpectedWebSocketError(err) {
					return
				}
				log.Printf("ws: close-now error: %v", err)
			}
		}()

		// CloseRead drains incoming frames and cancels ctx when the peer closes.
		ctx := conn.CloseRead(context.Background())
		pingTicker := time.NewTicker(websocketPingInterval)
		defer pingTicker.Stop()

		remoteAddr := remoteIP(r.RemoteAddr)
		_, ch, disconnect, unsub, err := h.Subscribe(claims.UserID, remoteAddr)
		if err != nil {
			status := websocket.StatusPolicyViolation
			reason := "websocket connection rejected"
			switch {
			case errors.Is(err, hub.ErrTooManyUserConnections):
				reason = "too many websocket connections for user"
			case errors.Is(err, hub.ErrTooManyIPConnections):
				reason = "too many websocket connections from ip"
			}
			if closeErr := conn.Close(status, reason); closeErr != nil {
				if !isExpectedWebSocketError(closeErr) {
					log.Printf("ws: close error: %v", closeErr)
				}
			} else {
				closeAttempted = true
			}
			return
		}
		defer unsub()

		for {
			select {
			case <-ctx.Done():
				if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
					if !isExpectedWebSocketError(err) {
						log.Printf("ws: close error: %v", err)
					}
				} else {
					closeAttempted = true
				}
				return
			case <-pingTicker.C:
				pingCtx, cancel := context.WithTimeout(context.Background(), websocketWriteTimeout)
				err := conn.Ping(pingCtx)
				cancel()
				if err != nil {
					if !isExpectedWebSocketError(err) {
						log.Printf("ws: ping error: %v", err)
					}
					return
				}
			case reason, ok := <-disconnect:
				if !ok {
					return
				}
				if reason == "" {
					reason = "session invalidated"
				}
				if err := conn.Close(websocket.StatusPolicyViolation, reason); err != nil {
					if !isExpectedWebSocketError(err) {
						log.Printf("ws: close error: %v", err)
					}
				} else {
					closeAttempted = true
				}
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				writeCtx, cancel := context.WithTimeout(context.Background(), websocketWriteTimeout)
				err := conn.Write(writeCtx, websocket.MessageText, msg)
				cancel()
				if err != nil {
					if !isExpectedWebSocketError(err) {
						log.Printf("ws: write error: %v", err)
					}
					return
				}
			}
		}
	}
}

func isExpectedWebSocketError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var closeErr websocket.CloseError
	if errors.As(err, &closeErr) {
		switch closeErr.Code {
		case websocket.StatusNormalClosure, websocket.StatusGoingAway, websocket.StatusNoStatusRcvd:
			return true
		}
	}
	return strings.Contains(err.Error(), "use of closed network connection")
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

// WSMessage is the envelope for all WebSocket broadcasts.
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// MarshalWSMessage serialises a WSMessage. Returns nil on error (caller skips broadcast).
func MarshalWSMessage(msgType string, data interface{}) []byte {
	b, err := json.Marshal(WSMessage{Type: msgType, Data: data})
	if err != nil {
		log.Printf("ws: marshal error: %v", err)
		return nil
	}
	return b
}
