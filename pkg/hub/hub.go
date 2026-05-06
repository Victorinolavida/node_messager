package hub

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
	"node_messager/pkg/dto"
	"node_messager/pkg/msgstore"
)

type Client struct {
	hub  *Hub
	conn net.Conn
	send chan []byte
}

type Dispatcher interface {
	Dispatch(msg dto.Message)
}

type Hub struct {
	name       string
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	log        *zap.SugaredLogger
	store      *msgstore.Store
	dispatcher Dispatcher
}

func (h *Hub) SetDispatcher(d Dispatcher) { h.dispatcher = d }

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

			if h.dispatcher != nil {
				go h.dispatcher.Dispatch(msg)
			}

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

func (h *Hub) Serve(conn net.Conn) {
	c := &Client{hub: h, conn: conn, send: make(chan []byte, 256)}
	h.register <- c
	go c.writePump()
	go c.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		if err := c.conn.Close(); err != nil {
			c.hub.log.Debugf("[%s] close error: %v", c.hub.name, err)
		}
	}()
	scanner := bufio.NewScanner(c.conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		buf := make([]byte, len(line))
		copy(buf, line)
		c.hub.broadcast <- buf
	}
}

func (c *Client) writePump() {
	defer func() {
		if err := c.conn.Close(); err != nil {
			c.hub.log.Debugf("[%s] close error: %v", c.hub.name, err)
		}
	}()
	for data := range c.send {
		if _, err := fmt.Fprintf(c.conn, "%s\n", data); err != nil {
			c.hub.log.Debugf("[%s] write error, closing connection: %v", c.hub.name, err)
			break
		}
		var msg dto.Message
		if err := json.Unmarshal(data, &msg); err == nil {
			c.hub.log.Debugf("[%s] ack   id=%s at=%s",
				c.hub.name, msg.ID, time.Now().UTC().Format(time.RFC3339))
		}
	}
}
