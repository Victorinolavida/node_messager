package entities

import "time"

type MessageType string

const (
	MSG       MessageType = "MSG"
	BROADCAST MessageType = "BROADCAST"
	ACK       MessageType = "ACK"
)

type NodeName string

type Timestamp struct {
	value time.Time
}

func NewTimestamp(value time.Time) Timestamp {
	return Timestamp{value}
}

func (t Timestamp) String() string {
	return t.value.UTC().Format(time.RFC3339)
}

func (t Timestamp) Value() string {
	return t.value.UTC().Format(time.RFC3339)
}
