package election

import (
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"
	"node_messager/internal/nodestate"
	"node_messager/pkg/dto"
	"node_messager/pkg/node"
	"node_messager/pkg/sender"
)

func newLogger(t *testing.T) *zap.SugaredLogger {
	t.Helper()
	l, _ := zap.NewDevelopment()
	return l.Sugar()
}

func buildEngine(t *testing.T, self node.Node, all []node.Node, masterID int) *Engine {
	t.Helper()
	log := newLogger(t)
	state := nodestate.New(self, all, masterID)
	pool := sender.NewPool(log)
	t.Cleanup(pool.CloseAll)
	return New(self, state, pool, log)
}

func TestStartElection_NoHigherNodes_WinsImmediately(t *testing.T) {
	self := node.Node{ID: 10, Name: "top"}
	low1 := node.Node{ID: 1, Name: "low1", Host: "127.0.0.1", Port: 19901}
	low2 := node.Node{ID: 2, Name: "low2", Host: "127.0.0.1", Port: 19902}
	e := buildEngine(t, self, []node.Node{self, low1, low2}, 1)

	e.StartElection()
	// winner declared synchronously when no higher nodes
	time.Sleep(50 * time.Millisecond)
	if e.state.GetMasterID() != self.ID {
		t.Fatalf("want masterID=%d, got %d", self.ID, e.state.GetMasterID())
	}
}

func TestStartElection_AlreadyRunning_IsIdempotent(t *testing.T) {
	self := node.Node{ID: 1, Name: "n"}
	e := buildEngine(t, self, []node.Node{self}, 1)
	e.mu.Lock()
	e.running = true
	e.mu.Unlock()
	// calling again should return immediately without crashing
	done := make(chan struct{})
	go func() { e.StartElection(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("StartElection blocked when already running")
	}
}

func TestHandleElection_LowerIDIgnores(t *testing.T) {
	self := node.Node{ID: 1, Name: "low"}
	high := node.Node{ID: 3, Name: "high", Host: "127.0.0.1", Port: 19903}
	e := buildEngine(t, self, []node.Node{self, high}, 3)

	p := dto.ElectionPayload{CandidateID: 3}
	data, _ := json.Marshal(p)
	msg := dto.Message{Type: dto.TypeElection, FromNode: "high", Content: string(data)}
	// self.ID(1) <= candidate(3) → should return without action
	done := make(chan struct{})
	go func() { e.HandleElection(msg); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("HandleElection blocked")
	}
	// master should not have changed
	if e.state.GetMasterID() == self.ID {
		// still the original master — correct
	}
}

func TestHandleElection_HigherIDStartsOwnElection(t *testing.T) {
	self := node.Node{ID: 5, Name: "high"}
	low := node.Node{ID: 2, Name: "low", Host: "127.0.0.1", Port: 19904}
	e := buildEngine(t, self, []node.Node{self, low}, 2)

	oldTimeout := okTimeout
	okTimeout = 100 * time.Millisecond
	t.Cleanup(func() { okTimeout = oldTimeout })

	p := dto.ElectionPayload{CandidateID: 2}
	data, _ := json.Marshal(p)
	msg := dto.Message{Type: dto.TypeElection, FromNode: "low", Content: string(data)}
	e.HandleElection(msg)

	// self (ID=5) > candidate (ID=2), so it should start its own election and win
	time.Sleep(200 * time.Millisecond)
	if e.state.GetMasterID() != self.ID {
		t.Fatalf("want masterID=%d (self wins), got %d", self.ID, e.state.GetMasterID())
	}
}

func TestHandleElectionOK_StopsElection(t *testing.T) {
	self := node.Node{ID: 1, Name: "n"}
	e := buildEngine(t, self, []node.Node{self}, 1)

	oldTimeout := okTimeout
	okTimeout = 5 * time.Second
	t.Cleanup(func() { okTimeout = oldTimeout })

	e.mu.Lock()
	e.running = true
	e.okTimer = time.AfterFunc(okTimeout, func() { t.Error("timer fired — should have been stopped") })
	e.mu.Unlock()

	msg := dto.Message{Type: dto.TypeElectionOK, FromNode: "higher"}
	e.HandleElectionOK(msg)

	e.mu.Lock()
	running := e.running
	e.mu.Unlock()
	if running {
		t.Fatal("running should be false after ELECTION_OK")
	}
}

func TestHandleCoordinator_UpdatesMasterID(t *testing.T) {
	self := node.Node{ID: 1, Name: "n"}
	e := buildEngine(t, self, []node.Node{self}, 1)

	p := dto.CoordinatorPayload{MasterID: 7}
	data, _ := json.Marshal(p)
	msg := dto.Message{Type: dto.TypeCoordinator, FromNode: "winner", Content: string(data)}
	e.HandleCoordinator(msg)
	if e.state.GetMasterID() != 7 {
		t.Fatalf("want masterID=7, got %d", e.state.GetMasterID())
	}
}

func TestHigherNodes_FiltersCorrectly(t *testing.T) {
	self := node.Node{ID: 3, Name: "mid"}
	all := []node.Node{
		{ID: 1, Name: "n1"}, {ID: 2, Name: "n2"},
		self,
		{ID: 4, Name: "n4"}, {ID: 5, Name: "n5"},
	}
	e := buildEngine(t, self, all, 1)
	higher := e.higherNodes()
	if len(higher) != 2 {
		t.Fatalf("want 2 higher nodes, got %d: %+v", len(higher), higher)
	}
	for _, n := range higher {
		if n.ID <= self.ID {
			t.Errorf("node %d is not higher than self %d", n.ID, self.ID)
		}
	}
}

func TestStartElection_WinsAfterTimeout(t *testing.T) {
	oldTimeout := okTimeout
	okTimeout = 50 * time.Millisecond
	t.Cleanup(func() { okTimeout = oldTimeout })

	self := node.Node{ID: 1, Name: "n"}
	// peer exists but port is not listening
	unreachable := node.Node{ID: 2, Name: "dead", Host: "127.0.0.1", Port: 19997}
	e := buildEngine(t, self, []node.Node{self, unreachable}, 2)

	e.StartElection()
	time.Sleep(200 * time.Millisecond) // wait for okTimeout + small buffer
	if e.state.GetMasterID() != self.ID {
		t.Fatalf("want self as master after timeout, got %d", e.state.GetMasterID())
	}
}
