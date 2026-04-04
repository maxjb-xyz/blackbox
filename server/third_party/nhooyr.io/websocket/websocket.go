package websocket

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type MessageType int

const (
	MessageText MessageType = 1
)

type StatusCode int

const (
	StatusNormalClosure StatusCode = 1000
)

type AcceptOptions struct {
	InsecureSkipVerify bool
}

type Conn struct {
	conn    net.Conn
	bufrw   *bufio.ReadWriter
	writeMu sync.Mutex
	closeMu sync.Once
}

func Accept(w http.ResponseWriter, r *http.Request, opts *AcceptOptions) (*Conn, error) {
	if !headerContainsToken(r.Header, "Connection", "Upgrade") || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return nil, errors.New("websocket upgrade required")
	}
	if opts == nil || !opts.InsecureSkipVerify {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !sameOriginHost(u.Host, r.Host) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return nil, errors.New("origin not allowed")
			}
		}
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		http.Error(w, "missing websocket key", http.StatusBadRequest)
		return nil, errors.New("missing websocket key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket hijack unavailable", http.StatusInternalServerError)
		return nil, errors.New("http hijacker unavailable")
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	if err := writeHandshake(bufrw, key); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &Conn{conn: conn, bufrw: bufrw}, nil
}

func (c *Conn) CloseNow() {
	c.closeMu.Do(func() {
		_ = c.conn.Close()
	})
}

func (c *Conn) Close(status StatusCode, reason string) error {
	payload := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(payload[:2], uint16(status))
	copy(payload[2:], reason)
	_ = c.writeFrame(0x8, payload, time.Time{})
	c.CloseNow()
	return nil
}

func (c *Conn) CloseRead(ctx context.Context) context.Context {
	readCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		buf := make([]byte, 1024)
		for {
			if _, err := c.conn.Read(buf); err != nil {
				return
			}
		}
	}()
	return readCtx
}

func (c *Conn) Write(ctx context.Context, typ MessageType, msg []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.Time{}
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	return c.writeFrame(frameOpcode(typ), msg, deadline)
}

func (c *Conn) writeFrame(opcode byte, payload []byte, deadline time.Time) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if !deadline.IsZero() {
		if err := c.conn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		defer c.conn.SetWriteDeadline(time.Time{})
	}
	header := []byte{0x80 | opcode}
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) <= 65535:
		header = append(header, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		ext := make([]byte, 8)
		binary.BigEndian.PutUint64(ext, uint64(len(payload)))
		header = append(header, 127)
		header = append(header, ext...)
	}
	if _, err := c.bufrw.Write(header); err != nil {
		return err
	}
	if _, err := c.bufrw.Write(payload); err != nil {
		return err
	}
	return c.bufrw.Flush()
}

func frameOpcode(typ MessageType) byte {
	switch typ {
	case MessageText:
		return 0x1
	default:
		return 0x2
	}
}

func writeHandshake(bufrw *bufio.ReadWriter, key string) error {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	if _, err := bufrw.WriteString("HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		return err
	}
	if _, err := bufrw.WriteString("Upgrade: websocket\r\n"); err != nil {
		return err
	}
	if _, err := bufrw.WriteString("Connection: Upgrade\r\n"); err != nil {
		return err
	}
	if _, err := bufrw.WriteString("Sec-WebSocket-Accept: " + accept + "\r\n\r\n"); err != nil {
		return err
	}
	return bufrw.Flush()
}

func headerContainsToken(h http.Header, key, token string) bool {
	for _, value := range h.Values(key) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func sameOriginHost(originHost, requestHost string) bool {
	return strings.EqualFold(strings.TrimSpace(originHost), strings.TrimSpace(requestHost))
}
