package dispatcher

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"
	"node_messager/internal/consensus"
	"node_messager/internal/election"
	"node_messager/internal/heartbeat"
	"node_messager/internal/mutex"
	"node_messager/internal/nodestate"
	"node_messager/internal/service"
	"node_messager/pkg/dto"
)

// NodeDispatcher implements hub.Dispatcher and routes each message type.
type NodeDispatcher struct {
	state     *nodestate.State
	consensus *consensus.Engine
	mutex     *mutex.Engine
	election  *election.Engine
	heartbeat *heartbeat.Monitor
	svc       *service.TicketService
	log       *zap.SugaredLogger
	ctx       context.Context
}

func New(
	ctx context.Context,
	state *nodestate.State,
	cons *consensus.Engine,
	mtx *mutex.Engine,
	elec *election.Engine,
	hb *heartbeat.Monitor,
	svc *service.TicketService,
	log *zap.SugaredLogger,
) *NodeDispatcher {
	return &NodeDispatcher{
		state:     state,
		consensus: cons,
		mutex:     mtx,
		election:  elec,
		heartbeat: hb,
		svc:       svc,
		log:       log,
		ctx:       ctx,
	}
}

// Dispatch is called by hub.Run() in a goroutine for every received message.
func (d *NodeDispatcher) Dispatch(msg dto.Message) {
	switch msg.Type {
	// ── Heartbeat ──────────────────────────────────────────────────────────
	case dto.TypePing:
		d.heartbeat.HandlePing(msg)
	case dto.TypePong:
		d.heartbeat.HandlePong(msg)

	// ── Consensus ──────────────────────────────────────────────────────────
	case dto.TypePropose:
		d.consensus.HandlePropose(msg)
	case dto.TypeVoteYes, dto.TypeVoteNo:
		d.consensus.HandleVote(msg)
	case dto.TypeCommit:
		d.consensus.HandleCommit(msg)

	// ── Mutual exclusion ───────────────────────────────────────────────────
	case dto.TypeLockRequest:
		d.mutex.HandleLockRequest(msg)
	case dto.TypeLockGrant:
		d.mutex.HandleLockGrant(msg)
	case dto.TypeLockDeny:
		d.mutex.HandleLockDeny(msg)
	case dto.TypeLockRelease:
		d.mutex.HandleLockRelease(msg)

	// ── Leader election ────────────────────────────────────────────────────
	case dto.TypeElection:
		d.election.HandleElection(msg)
	case dto.TypeElectionOK:
		d.election.HandleElectionOK(msg)
	case dto.TypeCoordinator:
		d.election.HandleCoordinator(msg)

	// ── Data queries ───────────────────────────────────────────────────────
	case dto.TypeQuery:
		d.svc.HandleQuery(msg)
	case dto.TypeQueryResponse:
		d.svc.HandleQueryResponse(msg)

	// ── Device distribution ────────────────────────────────────────────────
	case dto.TypeAddDevice:
		go d.svc.HandleAddDevice(d.ctx, msg)

	// ── Node failure ───────────────────────────────────────────────────────
	case dto.TypeNodeDead:
		if d.state.IsMaster() {
			var p dto.NodeDeadPayload
			if err := json.Unmarshal([]byte(msg.Content), &p); err == nil {
				go d.svc.RedistributeTickets(d.ctx, p.DeadNodeID)
			}
		}

	// TypeMsg, TypeBroadcast, TypeRedistribute, TypeAssignTicket, TypeCloseTicket
	// pass through — handled by hub fan-out or CLI
	default:
		// no application handler needed
	}
}
