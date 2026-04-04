package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/hub"
	"github.com/coder/websocket"
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
		defer func() {
			if err := conn.CloseNow(); err != nil {
				log.Printf("ws: close-now error: %v", err)
			}
		}()

		// CloseRead drains incoming frames and cancels ctx when the peer closes.
		ctx := conn.CloseRead(context.Background())

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
				log.Printf("ws: close error: %v", closeErr)
			}
			return
		}
		defer unsub()

		for {
			select {
			case <-ctx.Done():
				if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
					log.Printf("ws: close error: %v", err)
				}
				return
			case reason, ok := <-disconnect:
				if !ok {
					return
				}
				if reason == "" {
					reason = "session invalidated"
				}
				if err := conn.Close(websocket.StatusPolicyViolation, reason); err != nil {
					log.Printf("ws: close error: %v", err)
				}
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
					log.Printf("ws: write error: %v", err)
					return
				}
			}
		}
	}
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
