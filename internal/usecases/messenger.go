package usecases

import (
	"node_messager/internal/entities"
	secondaryports "node_messager/internal/ports/secondary"
)

type MessengerUseCase struct {
	ws secondaryports.WebsocketPort
}

func NewMessengerUseCase(ws secondaryports.WebsocketPort) *MessengerUseCase {
	return &MessengerUseCase{ws: ws}
}

func (m *MessengerUseCase) SendMessage(to entities.NodeName, content string) error {
	return nil
}

func (m *MessengerUseCase) Broadcast(content string) error {
	return nil
}

func (m *MessengerUseCase) ShowLogs() ([]entities.Message, error) {
	return nil, nil
}
