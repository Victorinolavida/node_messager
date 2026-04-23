package entities

import "github.com/google/uuid"

type Message struct {
	ID        uuid.UUID
	Type      MessageType
	FromNode  NodeName
	ToNode    NodeName
	Content   string
	CreatedAt Timestamp
}
