package heartbeat

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"go.uber.org/zap"
	"node_messager/internal/election"
	"node_messager/internal/nodestate"
	"node_messager/pkg/dto"
	"node_messager/pkg/node"
	"node_messager/pkg/sender"
)

const (
	pingInterval = 5 * time.Second
	pongWait     = 2 * time.Second
	maxMissed    = 3
)

type Monitor struct {
	self     node.Node
	state    *nodestate.State
	pool     *sender.Pool
	election *election.Engine
	log      *zap.SugaredLogger

	mu      sync.Mutex
	missed  map[int]int
	pongChs map[int]chan struct{}

	// OnNodeDead is called when a node is declared dead (for redistribution).
	OnNodeDead func(deadNodeID int)
}

func New(self node.Node, state *nodestate.State, pool *sender.Pool, elec *election.Engine, log *zap.SugaredLogger) *Monitor {
	return &Monitor{
		self:     self,
		state:    state,
		pool:     pool,
		election: elec,
		log:      log,
		missed:   make(map[int]int),
		pongChs:  make(map[int]chan struct{}),
	}
}

func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pingAll(ctx)
		}
	}
}

func (m *Monitor) pingAll(ctx context.Context) {
	for _, peer := range m.state.Peers() {
		go m.pingOne(ctx, peer)
	}
}

func (m *Monitor) pingOne(ctx context.Context, peer node.Node) {
	ch := make(chan struct{}, 1)
	m.mu.Lock()
	m.pongChs[peer.ID] = ch
	m.mu.Unlock()

	_ = m.pool.Send(m.self, peer, dto.TypePing, "")

	select {
	case <-ch:
		m.mu.Lock()
		m.missed[peer.ID] = 0
		m.mu.Unlock()
		m.state.MarkAlive(peer.ID)
	case <-time.After(pongWait):
		m.mu.Lock()
		m.missed[peer.ID]++
		missed := m.missed[peer.ID]
		m.mu.Unlock()
		if missed >= maxMissed {
			m.declareDead(peer.ID)
		}
	case <-ctx.Done():
	}

	m.mu.Lock()
	delete(m.pongChs, peer.ID)
	m.mu.Unlock()
}

func (m *Monitor) declareDead(id int) {
	if !m.state.IsAlive(id) {
		return // already declared dead
	}
	m.state.MarkDead(id)
	m.log.Warnf("[heartbeat] node %d declared dead", id)

	masterID := m.state.GetMasterID()
	if id == masterID {
		m.log.Warnf("[heartbeat] master is dead — starting election")
		go m.election.StartElection()
	} else if m.state.IsMaster() {
		// we are master; handle redistribution
		if m.OnNodeDead != nil {
			go m.OnNodeDead(id)
		}
	} else {
		// notify master
		peer := m.state.GetMasterNode()
		if peer != nil {
			p := dto.NodeDeadPayload{DeadNodeID: id}
			data, _ := json.Marshal(p)
			_ = m.pool.Send(m.self, *peer, dto.TypeNodeDead, string(data))
		}
	}
}

// HandlePing replies with PONG.
func (m *Monitor) HandlePing(msg dto.Message) {
	peer := m.state.NodeByName(msg.FromNode)
	if peer == nil {
		return
	}
	_ = m.pool.Send(m.self, *peer, dto.TypePong, "")
}

// HandlePong signals the waiting pingOne goroutine.
func (m *Monitor) HandlePong(msg dto.Message) {
	peer := m.state.NodeByName(msg.FromNode)
	if peer == nil {
		return
	}
	m.mu.Lock()
	ch, ok := m.pongChs[peer.ID]
	m.mu.Unlock()
	if ok {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
