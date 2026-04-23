package websocket

import (
	"node_messager/internal/entities"
)

type WebsocketAdapter struct {
	url  string
	conn any
}

func NewWebsocketAdapter(url string) *WebsocketAdapter {
	return &WebsocketAdapter{url: url}
}

func (w *WebsocketAdapter) Connect(url string) error {
	return nil
}

func (w *WebsocketAdapter) Send(msg entities.Message) error {
	return nil
}

func (w *WebsocketAdapter) Receive() (<-chan entities.Message, error) {
	return nil, nil
}

func (w *WebsocketAdapter) Disconnect() error {
	return nil
}
