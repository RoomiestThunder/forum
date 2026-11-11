// Package websocket pushes live events to connected users.
//
// A client authenticates by sending its access token as the first frame of the
// connection rather than in the URL: URLs end up in proxy and access logs, and
// a token there is a credential leaked to anyone who can read them.
package websocket

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"forum/internal/metrics"
	"forum/internal/models"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 50 * time.Second
	maxMessageSize = 512

	// defaultAuthTimeout bounds how long a client may hold a goroutine and a
	// socket open without authenticating.
	defaultAuthTimeout = 5 * time.Second
	maxAuthFrame       = 4096

	// sendBuffer is how many events may queue for one client. A client that
	// falls this far behind is dropped rather than allowed to stall delivery.
	sendBuffer = 256
)

// readyFrame tells the client its token was accepted and events will follow.
var readyFrame = []byte(`{"type":"ready"}`)

// Authenticator resolves an access token to a user id, or rejects it.
type Authenticator func(token string) (userID int, err error)

var errBadHandshake = errors.New("first frame must be {\"type\":\"auth\",\"token\":\"...\"}")

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Any origin may connect because the socket carries no ambient
	// credentials: it authenticates with a token the page must supply, never a
	// cookie, so a hostile site has nothing to ride on (no cross-site
	// websocket hijacking). An origin allow-list would add nothing but a way
	// to break the Vite dev proxy.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Hub tracks open connections per user and delivers events to them.
//
// The connection set is guarded by a mutex rather than funnelled through a
// dispatcher goroutine: delivery is a non-blocking send into each client's
// buffer, so there is nothing to wait on, and callers of Notify can never be
// stalled by the hub.
type Hub struct {
	mu      sync.RWMutex
	clients map[int]map[*Client]struct{} // userID -> connections
	closed  bool

	authTimeout time.Duration
	logger      *slog.Logger
}

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients:     make(map[int]map[*Client]struct{}),
		authTimeout: defaultAuthTimeout,
		logger:      logger,
	}
}

// Notify queues an event for every open connection of userID. It never
// blocks: a connection whose buffer is full is dropped.
func (h *Hub) Notify(userID int, notifType string, payload interface{}) {
	data, err := json.Marshal(&models.Notification{Type: notifType, UserID: userID, Payload: payload})
	if err != nil {
		h.logger.Error("ws marshal failed", slog.String("type", notifType), slog.Any("err", err))
		return
	}

	var slow []*Client
	h.mu.RLock()
	for c := range h.clients[userID] {
		select {
		case c.send <- data:
		default:
			slow = append(slow, c)
		}
	}
	h.mu.RUnlock()

	// Sends happen under the read lock and a channel is only closed under the
	// write lock, so a send can never hit a closed channel. Removal is done
	// after releasing the read lock, which is why it is a separate step.
	for _, c := range slow {
		h.logger.Warn("ws client too slow, dropping", slog.Int("user_id", userID))
		h.remove(c)
	}
}

// Disconnect closes every connection of a user, e.g. when they are banned.
func (h *Hub) Disconnect(userID int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients[userID] {
		h.dropLocked(c)
	}
}

// Close disconnects everyone and refuses new connections.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for _, set := range h.clients {
		for c := range set {
			h.dropLocked(c)
		}
	}
}

func (h *Hub) add(c *Client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false
	}
	set := h.clients[c.userID]
	if set == nil {
		set = make(map[*Client]struct{})
		h.clients[c.userID] = set
	}
	set[c] = struct{}{}
	metrics.ActiveWebSockets.Inc()
	return true
}

// remove is idempotent: a connection can be removed by the read loop, by a
// slow-consumer drop, and by a ban at the same time, and the gauge and the
// channel close must each happen exactly once.
func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dropLocked(c)
}

func (h *Hub) dropLocked(c *Client) {
	set := h.clients[c.userID]
	if _, ok := set[c]; !ok {
		return
	}
	delete(set, c)
	if len(set) == 0 {
		delete(h.clients, c.userID)
	}
	close(c.send) // the write loop sees this, sends a close frame and exits
	metrics.ActiveWebSockets.Dec()
}

// ServeWS upgrades the request, waits for the client to authenticate with its
// first frame, then registers the connection. A client that fails to
// authenticate in time is closed with a policy-violation status.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, authenticate Authenticator) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has already written the HTTP error response.
		h.logger.Debug("ws upgrade failed", slog.Any("err", err))
		return
	}

	userID, err := handshake(conn, authenticate, h.authTimeout)
	if err != nil {
		h.logger.Debug("ws handshake rejected", slog.Any("err", err))
		closeWith(conn, websocket.ClosePolicyViolation, "unauthorized")
		return
	}

	c := &Client{hub: h, conn: conn, send: make(chan []byte, sendBuffer), userID: userID}
	if !h.add(c) {
		closeWith(conn, websocket.CloseGoingAway, "server shutting down")
		return
	}
	c.send <- readyFrame // empty buffer, cannot block
	go c.writePump()
	go c.readPump()
}

func handshake(conn *websocket.Conn, authenticate Authenticator, timeout time.Duration) (int, error) {
	conn.SetReadLimit(maxAuthFrame)
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return 0, err
	}
	var frame struct {
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if err := conn.ReadJSON(&frame); err != nil {
		return 0, err
	}
	if frame.Type != "auth" || frame.Token == "" {
		return 0, errBadHandshake
	}
	return authenticate(frame.Token)
}

func closeWith(conn *websocket.Conn, code int, reason string) {
	msg := websocket.FormatCloseMessage(code, reason)
	_ = conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(writeWait))
	_ = conn.Close()
}

// Client is one authenticated WebSocket connection.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	userID int
}

// readPump exists to service control frames (pongs, close) and to notice a
// dead connection; clients have nothing to say after the handshake, so
// anything they send is discarded.
func (c *Client) readPump() {
	defer func() {
		c.hub.remove(c)
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
