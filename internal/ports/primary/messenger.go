package primaryports

import "node_messager/internal/entities"

type MessengerPort interface {
	SendMessage(to entities.NodeName, content string) error
	Broadcast(content string) error
	ShowLogs() ([]entities.Message, error)
}
