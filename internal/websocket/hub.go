package websocket

import (
	"context"
	"encoding/json"
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
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:    func(r *http.Request) bool { return true },
}

// Hub maintains a set of active clients and broadcasts notifications.
type Hub struct {
	mu      sync.RWMutex
	clients map[int][]*Client // userID → connections

	broadcast  chan *models.Notification
	register   chan *Client
	unregister chan *Client

	logger *slog.Logger
}

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients:    make(map[int][]*Client),
		broadcast:  make(chan *models.Notification, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     logger,
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.userID] = append(h.clients[client.userID], client)
			h.mu.Unlock()
			metrics.ActiveWebSockets.Inc()
			h.logger.Info("ws client registered", slog.Int("user_id", client.userID))

		case client := <-h.unregister:
			h.mu.Lock()
			conns := h.clients[client.userID]
			for i, c := range conns {
				if c == client {
					h.clients[client.userID] = append(conns[:i], conns[i+1:]...)
					close(client.send)
					break
				}
			}
			if len(h.clients[client.userID]) == 0 {
				delete(h.clients, client.userID)
			}
			h.mu.Unlock()
			metrics.ActiveWebSockets.Dec()

		case notification := <-h.broadcast:
			h.mu.RLock()
			clients := h.clients[notification.UserID]
			h.mu.RUnlock()
			data, err := json.Marshal(notification)
			if err != nil {
				continue
			}
			for _, client := range clients {
				select {
				case client.send <- data:
				default:
					// Slow client: schedule removal via the unregister path so
					// the channel is closed exactly once and the clients map is
					// updated consistently.
					go func(c *Client) { h.unregister <- c }(client)
				}
			}
		}
	}
}

// Notify sends a notification to a specific user.
func (h *Hub) Notify(userID int, notifType string, payload interface{}) {
	h.broadcast <- &models.Notification{
		Type:    notifType,
		UserID:  userID,
		Payload: payload,
	}
}

// ServeWS upgrades the connection and registers the client.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, userID int) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("ws upgrade failed", slog.Any("err", err))
		return
	}
	client := &Client{hub: h, conn: conn, send: make(chan []byte, 256), userID: userID}
	h.register <- client
	go client.writePump()
	go client.readPump()
}

// Client represents a WebSocket connection.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	userID int
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
