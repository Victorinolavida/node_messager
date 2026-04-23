package wsclient

import (
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

type Client struct {
	mu   sync.Mutex
	conn *websocket.Conn
	Recv chan []byte
}

func Connect(host string, port int) (*Client, error) {
	url := fmt.Sprintf("ws://%s:%d/ws", host, port)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", url, err)
	}
	c := &Client{
		conn: conn,
		Recv: make(chan []byte, 256),
	}
	go c.readLoop()
	return c, nil
}

func (c *Client) Send(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, msg)
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) readLoop() {
	defer close(c.Recv)
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		c.Recv <- msg
	}
}
