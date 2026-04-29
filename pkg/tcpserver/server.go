package tcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
	"node_messager/pkg/dto"
	"node_messager/pkg/logger"
	"node_messager/pkg/msgstore"
	"node_messager/pkg/node"
)

type client struct {
	hub  *hub
	conn net.Conn
	send chan []byte
}

type hub struct {
	name       string
	clients    map[*client]bool
	broadcast  chan []byte
	register   chan *client
	unregister chan *client
	log        *zap.SugaredLogger
	store      *msgstore.Store
}

func newHub(name string, log *zap.SugaredLogger, store *msgstore.Store) *hub {
	return &hub{
		name:       name,
		clients:    make(map[*client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *client),
		unregister: make(chan *client),
		log:        log,
		store:      store,
	}
}

func (h *hub) run() {
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

func (c *client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	scanner := bufio.NewScanner(c.conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		cp := make([]byte, len(line))
		copy(cp, line)
		c.hub.broadcast <- cp
	}
}

func (c *client) writePump() {
	defer c.conn.Close()
	for data := range c.send {
		if _, err := fmt.Fprintf(c.conn, "%s\n", data); err != nil {
			break
		}
		var msg dto.Message
		if err := json.Unmarshal(data, &msg); err == nil {
			c.hub.log.Debugf("[%s] ack   id=%s at=%s",
				c.hub.name, msg.ID, time.Now().UTC().Format(time.RFC3339))
		}
	}
}

type Server struct {
	node  node.Node
	store *msgstore.Store
}

func New(n node.Node, store *msgstore.Store) *Server {
	return &Server{node: n, store: store}
}

func (s *Server) Start(ctx context.Context) error {
	l := logger.GetContextLogger(ctx)
	displayAddr := fmt.Sprintf("%s:%d", s.node.Host, s.node.Port)
	bindAddr := fmt.Sprintf(":%d", s.node.Port)
	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", bindAddr, err)
	}
	h := newHub(s.node.Name, l, s.store)
	go h.run()
	l.Infof("[%s] listening on %s", s.node.Name, displayAddr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return fmt.Errorf("accept: %w", err)
		}
		c := &client{hub: h, conn: conn, send: make(chan []byte, 256)}
		h.register <- c
		go c.writePump()
		go c.readPump()
	}
}
