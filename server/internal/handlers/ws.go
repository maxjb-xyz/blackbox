package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"blackbox/server/internal/hub"
	"github.com/coder/websocket"
)

func WebSocketHandler(h *hub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		// CloseRead pumps incoming frames (required by nhooyr) and cancels ctx on close.
		ctx := conn.CloseRead(context.Background())

		_, ch, unsub := h.Subscribe()
		defer unsub()

		for {
			select {
			case <-ctx.Done():
				if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
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
