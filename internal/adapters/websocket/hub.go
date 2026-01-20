package websocket

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Client represents a websocket connection
type Client struct {
	hub       *Hub
	conn      *websocket.Conn
	send      chan []byte
	userID    string
	userRole  string
	userEmail string
	userName  string
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients      map[*Client]bool
	broadcast    chan []byte
	register     chan *Client
	unregister   chan *Client
	mu           sync.RWMutex
	adminClients map[string]*Client
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:      make(map[*Client]bool),
		broadcast:    make(chan []byte, 256),
		register:     make(chan *Client),
		unregister:   make(chan *Client),
		adminClients: make(map[string]*Client),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			if client.userRole == "ADMIN" {
				h.adminClients[client.userID] = client
				log.Printf("✅ Admin/Nurse connected: %s (%s) - UserID: %s (Total: %d)",
					client.userName, client.userEmail, client.userID, len(h.adminClients))
			}
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if client.userRole == "ADMIN" {
					delete(h.adminClients, client.userID)
					log.Printf("Admin/Nurse disconnected: %s (%s) - UserID: %s (Total: %d)",
						client.userName, client.userEmail, client.userID, len(h.adminClients))
				}
				close(client.send)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
					if client.userRole == "ADMIN" {
						delete(h.adminClients, client.userID)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastToAdmins sends message only to connected ADMIN users (nurses)
func (h *Hub) BroadcastToAdmins(message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	sent := 0
	recipients := []string{}

	for userID, client := range h.adminClients {
		select {
		case client.send <- message:
			sent++
			recipients = append(recipients, client.userName+" ("+client.userEmail+")")
		default:
			log.Printf("Failed to send to admin/nurse %s, removing", client.userName)
			close(client.send)
			delete(h.clients, client)
			delete(h.adminClients, userID)
		}
	}

	if sent > 0 {
		log.Printf("📢 Broadcasted alert to %d admin/nurses: %v", sent, recipients)
	} else {
		log.Printf("⚠️  No connected admin/nurses to receive alert")
	}
}

// GetConnectedAdminCount returns number of connected ADMIN users
func (h *Hub) GetConnectedAdminCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.adminClients)
}


// readPump pumps messages from the websocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}

// writePump pumps messages from the hub to the websocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
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

// Upgrade upgrades HTTP connection to WebSocket
func Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (*websocket.Conn, error) {
	return upgrader.Upgrade(w, r, responseHeader)
}
