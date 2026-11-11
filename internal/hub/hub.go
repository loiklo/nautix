// Package hub broadcasts JSON messages to every connected WebSocket client.
package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type client struct {
	send   chan []byte
	cancel context.CancelFunc
}

// Hub keeps track of the connected clients.
type Hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
}

func New() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

// Broadcast sends v (encoded as JSON) to every client. A client that cannot
// keep up is disconnected; it will reconnect and reload the full state.
func (h *Hub) Broadcast(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.send <- b:
		default:
			c.cancel()
		}
	}
}

// Refresh tells every client to reload the application state.
func (h *Hub) Refresh() {
	h.Broadcast(map[string]string{"type": "refresh"})
}

// KickAll closes every connection. Clients reconnect on their own, which
// re-checks their session: used whenever sessions are revoked.
func (h *Hub) KickAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		c.cancel()
	}
}

// Serve upgrades the request to a WebSocket and pushes broadcasts to it until
// the connection ends. The Origin header must match the Host header.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	ctx = conn.CloseRead(ctx) // clients never send data; this handles close and pong frames

	c := &client{send: make(chan []byte, 64), cancel: cancel}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}()

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-c.send:
			wctx, done := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Write(wctx, websocket.MessageText, b)
			done()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, done := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Ping(pctx)
			done()
			if err != nil {
				return
			}
		}
	}
}
