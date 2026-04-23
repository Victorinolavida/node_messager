package msgstore

import (
	"sync"
	"time"

	"node_messager/pkg/dto"
)

type Entry struct {
	ReceivedAt time.Time
	Msg        dto.Message
}

type Store struct {
	mu      sync.Mutex
	entries []Entry
	max     int
}

func New(max int) *Store {
	return &Store{max: max, entries: make([]Entry, 0, max)}
}

func (s *Store) Save(msg dto.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, Entry{ReceivedAt: time.Now().UTC(), Msg: msg})
	if len(s.entries) > s.max {
		s.entries = s.entries[len(s.entries)-s.max:]
	}
	return nil
}

func (s *Store) Latest(n int) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n >= len(s.entries) {
		out := make([]Entry, len(s.entries))
		copy(out, s.entries)
		return out, nil
	}
	out := make([]Entry, n)
	copy(out, s.entries[len(s.entries)-n:])
	return out, nil
}
