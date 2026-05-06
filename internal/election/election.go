package election

import (
	"encoding/json"
	"sync"
	"time"

	"go.uber.org/zap"
	"node_messager/internal/nodestate"
	"node_messager/pkg/dto"
	"node_messager/pkg/node"
	"node_messager/pkg/sender"
)

var okTimeout = 3 * time.Second // var so tests can override

type Engine struct {
	self      node.Node
	state     *nodestate.State
	pool      *sender.Pool
	log       *zap.SugaredLogger
	mu        sync.Mutex
	running   bool
	okTimer   *time.Timer
}

func New(self node.Node, state *nodestate.State, pool *sender.Pool, log *zap.SugaredLogger) *Engine {
	return &Engine{self: self, state: state, pool: pool, log: log}
}

// StartElection initiates the Bully algorithm.
func (e *Engine) StartElection() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.mu.Unlock()

	e.log.Infof("[election] starting, self_id=%d", e.self.ID)

	higherNodes := e.higherNodes()
	if len(higherNodes) == 0 {
		// no higher nodes — self wins immediately
		e.declareVictory()
		return
	}

	p := dto.ElectionPayload{CandidateID: e.self.ID}
	for _, n := range higherNodes {
		_ = e.pool.SendJSON(e.self, n, dto.TypeElection, p)
	}

	e.mu.Lock()
	e.okTimer = time.AfterFunc(okTimeout, func() {
		// no one higher responded — self wins
		e.declareVictory()
	})
	e.mu.Unlock()
}

func (e *Engine) declareVictory() {
	e.mu.Lock()
	e.running = false
	if e.okTimer != nil {
		e.okTimer.Stop()
	}
	e.mu.Unlock()

	e.state.SetMasterID(e.self.ID)
	e.log.Infof("[election] won — new master id=%d", e.self.ID)

	p := dto.CoordinatorPayload{MasterID: e.self.ID}
	e.pool.BroadcastJSON(e.self, e.state.Peers(), dto.TypeCoordinator, p)
}

// HandleElection is called when this node receives a ELECTION message.
func (e *Engine) HandleElection(msg dto.Message) {
	var p dto.ElectionPayload
	if err := json.Unmarshal([]byte(msg.Content), &p); err != nil {
		return
	}

	if e.self.ID <= p.CandidateID {
		return // lower ID — ignore
	}

	// respond OK to sender
	sender := e.state.NodeByName(msg.FromNode)
	if sender != nil {
		reply := dto.ElectionPayload{CandidateID: e.self.ID}
		_ = e.pool.SendJSON(e.self, *sender, dto.TypeElectionOK, reply)
	}

	// start own election
	go e.StartElection()
}

// HandleElectionOK is called when a higher node responds — we are not the winner.
func (e *Engine) HandleElectionOK(msg dto.Message) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.running = false
	if e.okTimer != nil {
		e.okTimer.Stop()
		e.okTimer = nil
	}
	e.log.Infof("[election] received OK from %s, stepping down", msg.FromNode)
}

// HandleCoordinator is called when a new master announces itself.
func (e *Engine) HandleCoordinator(msg dto.Message) {
	var p dto.CoordinatorPayload
	if err := json.Unmarshal([]byte(msg.Content), &p); err != nil {
		return
	}
	e.state.SetMasterID(p.MasterID)
	e.log.Infof("[election] new master id=%d (announced by %s)", p.MasterID, msg.FromNode)
}

func (e *Engine) higherNodes() []node.Node {
	var out []node.Node
	for _, n := range e.state.AlivePeers() {
		if n.ID > e.self.ID {
			out = append(out, n)
		}
	}
	return out
}
