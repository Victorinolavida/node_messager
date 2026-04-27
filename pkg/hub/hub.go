package hub

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"node_messager/pkg/dto"
	"node_messager/pkg/msgstore"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

type Hub struct {
	name       string
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	log        *zap.SugaredLogger
	store      *msgstore.Store
}

func New(name string, log *zap.SugaredLogger, store *msgstore.Store) *Hub {
	return &Hub{
		name:       name,
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		log:        log,
		store:      store,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = true
			h.log.Debugf("[%s] client connected, total=%d", h.name, len(h.clients))

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
				h.log.Debugf("[%s] client disconnected, total=%d", h.name, len(h.clients))
			}

		case data := <-h.broadcast:
			var msg dto.Message
			if err := json.Unmarshal(data, &msg); err != nil {
				h.log.Warnf("[%s] invalid message payload: %v", h.name, err)
				continue
			}
			if err := h.store.Save(msg, msgstore.Received); err != nil {
				h.log.Warnf("[%s] store save: %v", h.name, err)
			}
			h.log.Infof("[%s] recv  type=%s from=%s to=%s id=%s — %q",
				h.name, msg.Type, msg.FromNode, msg.ToNode, msg.ID, msg.Content)

			for c := range h.clients {
				select {
				case c.send <- data:
				default:
					close(c.send)
					delete(h.clients, c)
				}
			}
		}
	}
}

func (h *Hub) ServeWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Errorf("[%s] ws upgrade failed: %v", h.name, err)
		return
	}
	c := &Client{hub: h, conn: conn, send: make(chan []byte, 256)}
	h.register <- c
	go c.writePump()
	go c.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		c.hub.broadcast <- data
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()
	for data := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			break
		}
		var msg dto.Message
		if err := json.Unmarshal(data, &msg); err == nil {
			c.hub.log.Debugf("[%s] ack   id=%s at=%s",
				c.hub.name, msg.ID, time.Now().UTC().Format(time.RFC3339))
		}
	}
}
