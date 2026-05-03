package ws

// Package ws provides the WebSocket hub — a driving adapter that manages
// real-time connections and rooms.  It also implements port.EventPort so
// application services can broadcast events without knowing about WebSockets.

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/di-eqa/backend/internal/domain/port"
	"github.com/gorilla/websocket"
)

// Hub manages all WebSocket clients and rooms.
type Hub struct {
	mu      sync.RWMutex
	rooms   map[string]map[*Client]bool
	clients map[*Client]bool
}

func NewHub() *Hub {
	return &Hub{
		rooms:   make(map[string]map[*Client]bool),
		clients: make(map[*Client]bool),
	}
}

// Broadcast implements port.EventPort — sends a domain event to a room.
func (h *Hub) Broadcast(room string, event port.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("ws marshal error: %v", err)
		return
	}
	h.mu.RLock()
	clients := make([]*Client, 0)
	if r, ok := h.rooms[room]; ok {
		for c := range r {
			clients = append(clients, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range clients {
		select {
		case c.send <- data:
		default:
		}
	}
}

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = true
}

func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		for room := range c.rooms {
			if r, ok := h.rooms[room]; ok {
				delete(r, c)
				if len(r) == 0 {
					delete(h.rooms, room)
				}
			}
		}
		close(c.send)
	}
}

func (h *Hub) JoinRoom(c *Client, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.rooms[room]; !ok {
		h.rooms[room] = make(map[*Client]bool)
	}
	h.rooms[room][c] = true
	c.rooms[room] = true
}

func (h *Hub) LeaveRoom(c *Client, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.rooms[room]; ok {
		delete(r, c)
		if len(r) == 0 {
			delete(h.rooms, room)
		}
	}
	delete(c.rooms, room)
}

func (h *Hub) RoomSize(room string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if r, ok := h.rooms[room]; ok {
		return len(r)
	}
	return 0
}

// ──────────────────────────────────────────────────────────────────────────
// Client
// ──────────────────────────────────────────────────────────────────────────

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 1024 * 8
)

// Client represents a single WebSocket connection.
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	rooms    map[string]bool
	userID   string
	username string
	fullName string
	hospital string
	mu       sync.Mutex
}

func NewClient(hub *Hub, conn *websocket.Conn, userID, username, fullName, hospital string) *Client {
	return &Client{
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, 64),
		rooms:    make(map[string]bool),
		userID:   userID,
		username: username,
		fullName: fullName,
		hospital: hospital,
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("ws read error: %v", err)
			}
			break
		}
		var msg struct {
			Type string          `json:"type"`
			Room string          `json:"room"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "join":
			if msg.Room != "" {
				c.hub.JoinRoom(c, msg.Room)
				c.hub.Broadcast(msg.Room, port.Event{
					Type: "presence",
					Payload: map[string]interface{}{
						"action":   "join",
						"userId":   c.userID,
						"username": c.username,
						"fullName": c.fullName,
						"hospital": c.hospital,
						"count":    c.hub.RoomSize(msg.Room),
					},
				})
			}
		case "leave":
			if msg.Room != "" {
				c.hub.LeaveRoom(c, msg.Room)
			}
		case "ping":
			b, _ := json.Marshal(port.Event{Type: "pong"})
			c.send <- b
		}
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.mu.Lock()
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				c.mu.Unlock()
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()
		case <-ticker.C:
			c.mu.Lock()
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()
		}
	}
}
