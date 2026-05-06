package sender

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"node_messager/pkg/dto"
	"node_messager/pkg/node"
	"node_messager/pkg/tcpclient"
)

// Pool manages reusable TCP connections to other nodes.
type Pool struct {
	mu    sync.Mutex
	conns map[int]*tcpclient.Client
	log   *zap.SugaredLogger
}

func NewPool(log *zap.SugaredLogger) *Pool {
	return &Pool{conns: make(map[int]*tcpclient.Client), log: log}
}

func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, c := range p.conns {
		_ = c.Close()
		delete(p.conns, id)
	}
}

func (p *Pool) get(n node.Node) (*tcpclient.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.conns[n.ID]; ok && !c.IsClosed() {
		return c, nil
	}
	c, err := tcpclient.Connect(n.Host, n.Port)
	if err != nil {
		return nil, err
	}
	p.conns[n.ID] = c
	return c, nil
}

// Send sends a typed message to target node.
func (p *Pool) Send(from node.Node, to node.Node, msgType, content string) error {
	c, err := p.get(to)
	if err != nil {
		return fmt.Errorf("connect %s: %w", to.Name, err)
	}
	m := dto.Message{
		ID:       uuid.New().String(),
		Type:     msgType,
		FromNode: from.Name,
		ToNode:   to.Name,
		Content:  content,
		SendAt:   time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return c.Send(data)
}

// SendJSON marshals payload to JSON then sends.
func (p *Pool) SendJSON(from node.Node, to node.Node, msgType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.Send(from, to, msgType, string(data))
}

// Broadcast sends to all targets, returns per-node errors.
func (p *Pool) Broadcast(from node.Node, targets []node.Node, msgType, content string) map[int]error {
	errs := make(map[int]error)
	for _, t := range targets {
		if err := p.Send(from, t, msgType, content); err != nil {
			p.log.Warnf("[sender] broadcast to %s failed: %v", t.Name, err)
			errs[t.ID] = err
		}
	}
	return errs
}

// BroadcastJSON marshals payload then broadcasts.
func (p *Pool) BroadcastJSON(from node.Node, targets []node.Node, msgType string, payload any) map[int]error {
	data, err := json.Marshal(payload)
	if err != nil {
		return map[int]error{-1: err}
	}
	return p.Broadcast(from, targets, msgType, string(data))
}
