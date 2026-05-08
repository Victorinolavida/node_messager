package mutex

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"node_messager/internal/nodestate"
	"node_messager/pkg/dto"
	"node_messager/pkg/node"
	"node_messager/pkg/sender"
)

var lockTimeout = 5 * time.Second // var so tests can override

const (
	acquireMaxRetries = 3
	acquireRetryDelay = 1 * time.Second
)

type pendingReq struct {
	requestID string
	fromNode  string
}

type Engine struct {
	self   node.Node
	state  *nodestate.State
	pool   *sender.Pool
	log    *zap.SugaredLogger
	mu     sync.Mutex
	holder string       // request_id of current holder, "" = free
	queue  []pendingReq // waiting requests
	// pending grants for self when acting as non-master requester
	grants map[string]chan bool
}

func New(self node.Node, state *nodestate.State, pool *sender.Pool, log *zap.SugaredLogger) *Engine {
	return &Engine{
		self:   self,
		state:  state,
		pool:   pool,
		log:    log,
		grants: make(map[string]chan bool),
	}
}

// Acquire blocks until the distributed lock is held. Retries on transient failure.
func (e *Engine) Acquire(ctx context.Context) (func(), error) {
	var (
		release func()
		lastErr error
	)
	for attempt := 1; attempt <= acquireMaxRetries; attempt++ {
		release, lastErr = e.acquire(ctx)
		if lastErr == nil {
			return release, nil
		}
		e.log.Warnf("[mutex] Acquire attempt=%d/%d failed: %v",
			attempt, acquireMaxRetries, lastErr)
		if attempt < acquireMaxRetries {
			select {
			case <-time.After(acquireRetryDelay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	e.log.Errorf("[mutex] Acquire failed after %d attempts: %v",
		acquireMaxRetries, lastErr)
	return nil, lastErr
}

func (e *Engine) acquire(ctx context.Context) (func(), error) {
	if e.state.IsMaster() {
		return e.acquireLocal()
	}
	return e.acquireRemote(ctx)
}

func (e *Engine) acquireLocal() (func(), error) {
	reqID := uuid.New().String()
	e.mu.Lock()
	if e.holder == "" {
		e.holder = reqID
		e.mu.Unlock()
		return func() { e.releaseLocal(reqID) }, nil
	}
	// queue self and wait synchronously (simplified: spin with short wait)
	e.queue = append(e.queue, pendingReq{requestID: reqID, fromNode: e.self.Name})
	e.mu.Unlock()

	// wait until granted
	ch := make(chan bool, 1)
	e.mu.Lock()
	e.grants[reqID] = ch
	e.mu.Unlock()

	select {
	case <-ch:
		return func() { e.releaseLocal(reqID) }, nil
	case <-time.After(lockTimeout):
		return nil, fmt.Errorf("mutex: local acquire timeout")
	}
}

func (e *Engine) releaseLocal(reqID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.holder != reqID {
		return
	}
	e.holder = ""
	if len(e.queue) > 0 {
		next := e.queue[0]
		e.queue = e.queue[1:]
		e.holder = next.requestID
		if ch, ok := e.grants[next.requestID]; ok {
			ch <- true
			delete(e.grants, next.requestID)
		} else {
			// remote node waiting — send LOCK_GRANT
			peer := e.state.NodeByName(next.fromNode)
			if peer != nil {
				p := dto.LockPayload{RequestID: next.requestID, Resource: "engineer_assignment"}
				_ = e.pool.SendJSON(e.self, *peer, dto.TypeLockGrant, p)
			}
		}
	}
}

func (e *Engine) acquireRemote(ctx context.Context) (func(), error) {
	master := e.state.GetMasterNode()
	if master == nil {
		return nil, fmt.Errorf("mutex: no master node found")
	}

	reqID := uuid.New().String()
	ch := make(chan bool, 1)

	e.mu.Lock()
	e.grants[reqID] = ch
	e.mu.Unlock()

	p := dto.LockPayload{RequestID: reqID, Resource: "engineer_assignment"}
	if err := e.pool.SendJSON(e.self, *master, dto.TypeLockRequest, p); err != nil {
		e.mu.Lock()
		delete(e.grants, reqID)
		e.mu.Unlock()
		return nil, fmt.Errorf("mutex: send lock request: %w", err)
	}

	select {
	case granted := <-ch:
		if !granted {
			return nil, fmt.Errorf("mutex: lock denied")
		}
		release := func() {
			p := dto.LockPayload{RequestID: reqID, Resource: "engineer_assignment"}
			_ = e.pool.SendJSON(e.self, *master, dto.TypeLockRelease, p)
		}
		return release, nil
	case <-time.After(lockTimeout):
		e.mu.Lock()
		delete(e.grants, reqID)
		e.mu.Unlock()
		return nil, fmt.Errorf("mutex: timeout waiting for lock grant")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// HandleLockRequest is called on the master when a peer wants the lock.
func (e *Engine) HandleLockRequest(msg dto.Message) {
	var p dto.LockPayload
	if err := json.Unmarshal([]byte(msg.Content), &p); err != nil {
		e.log.Warnf("[mutex] bad lock request: %v", err)
		return
	}

	e.mu.Lock()
	if e.holder == "" {
		e.holder = p.RequestID
		e.mu.Unlock()
		peer := e.state.NodeByName(msg.FromNode)
		if peer != nil {
			_ = e.pool.SendJSON(e.self, *peer, dto.TypeLockGrant, p)
		}
	} else {
		e.queue = append(e.queue, pendingReq{requestID: p.RequestID, fromNode: msg.FromNode})
		e.mu.Unlock()
		peer := e.state.NodeByName(msg.FromNode)
		if peer != nil {
			_ = e.pool.SendJSON(e.self, *peer, dto.TypeLockDeny, p)
		}
	}
}

// HandleLockGrant is called when this node receives LOCK_GRANT from master.
func (e *Engine) HandleLockGrant(msg dto.Message) {
	var p dto.LockPayload
	if err := json.Unmarshal([]byte(msg.Content), &p); err != nil {
		return
	}
	e.mu.Lock()
	ch, ok := e.grants[p.RequestID]
	e.mu.Unlock()
	if ok {
		ch <- true
	}
}

// HandleLockDeny is called when master says lock is busy — re-queue wait.
func (e *Engine) HandleLockDeny(msg dto.Message) {
	// DENY means queued on master; just keep waiting (grant will arrive later)
}

// HandleLockRelease is called on master when a peer releases the lock.
func (e *Engine) HandleLockRelease(msg dto.Message) {
	var p dto.LockPayload
	if err := json.Unmarshal([]byte(msg.Content), &p); err != nil {
		return
	}
	e.releaseLocal(p.RequestID)
}
