package msgstore

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"

	"node_messager/pkg/dto"
)

type EntryType string

const (
	Sent     EntryType = "sent"
	Received EntryType = "received"
)

type Entry struct {
	At   time.Time   `json:"at"`
	Type EntryType   `json:"type"`
	Msg  dto.Message `json:"msg"`
}

type Store struct {
	mu      sync.Mutex
	entries []Entry
	max     int
	file    *os.File
}

func New(max int) *Store {
	return &Store{max: max, entries: make([]Entry, 0, max)}
}

func NewWithFile(max int, path string) (*Store, error) {
	existing := loadFromFile(path, max)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &Store{max: max, entries: existing, file: f}, nil
}

func loadFromFile(path string, max int) []Entry {
	f, err := os.Open(path)
	if err != nil {
		return make([]Entry, 0, max)
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if len(entries) > max {
		entries = entries[len(entries)-max:]
	}
	return entries
}

func (s *Store) Save(msg dto.Message, t EntryType) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := Entry{At: time.Now().UTC(), Type: t, Msg: msg}
	s.entries = append(s.entries, entry)
	if len(s.entries) > s.max {
		s.entries = s.entries[len(s.entries)-s.max:]
	}
	if s.file != nil {
		line, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		line = append(line, '\n')
		_, err = s.file.Write(line)
		return err
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
