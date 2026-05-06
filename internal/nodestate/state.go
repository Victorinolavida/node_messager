package nodestate

import (
	"sync"

	"node_messager/pkg/node"
)

type State struct {
	mu       sync.RWMutex
	self     node.Node
	all      []node.Node
	masterID int
	alive    map[int]bool
}

func New(self node.Node, all []node.Node, masterID int) *State {
	alive := make(map[int]bool, len(all))
	for _, n := range all {
		alive[n.ID] = true
	}
	return &State{self: self, all: all, masterID: masterID, alive: alive}
}

func (s *State) Self() node.Node { return s.self }

func (s *State) All() []node.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]node.Node, len(s.all))
	copy(out, s.all)
	return out
}

func (s *State) Peers() []node.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []node.Node
	for _, n := range s.all {
		if n.ID != s.self.ID {
			out = append(out, n)
		}
	}
	return out
}

func (s *State) AlivePeers() []node.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []node.Node
	for _, n := range s.all {
		if n.ID != s.self.ID && s.alive[n.ID] {
			out = append(out, n)
		}
	}
	return out
}

func (s *State) AliveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, alive := range s.alive {
		if alive {
			n++
		}
	}
	return n
}

func (s *State) MarkAlive(id int) {
	s.mu.Lock()
	s.alive[id] = true
	s.mu.Unlock()
}

func (s *State) MarkDead(id int) {
	s.mu.Lock()
	s.alive[id] = false
	s.mu.Unlock()
}

func (s *State) IsAlive(id int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.alive[id]
}

func (s *State) GetMasterID() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.masterID
}

func (s *State) SetMasterID(id int) {
	s.mu.Lock()
	s.masterID = id
	s.mu.Unlock()
}

func (s *State) IsMaster() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.self.ID == s.masterID
}

func (s *State) GetMasterNode() *node.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, n := range s.all {
		if n.ID == s.masterID {
			cp := n
			return &cp
		}
	}
	return nil
}

func (s *State) NodeByName(name string) *node.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, n := range s.all {
		if n.Name == name {
			cp := n
			return &cp
		}
	}
	return nil
}

func (s *State) NodeByID(id int) *node.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, n := range s.all {
		if n.ID == id {
			cp := n
			return &cp
		}
	}
	return nil
}
