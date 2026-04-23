package secondaryports

import "node_messager/internal/entities"

type WebsocketPort interface {
	Connect(url string) error
	Send(msg entities.Message) error
	Receive() (<-chan entities.Message, error)
	Disconnect() error
}
