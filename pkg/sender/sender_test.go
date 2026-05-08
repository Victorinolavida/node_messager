package sender

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"node_messager/pkg/dto"
	"node_messager/pkg/hub"
	"node_messager/pkg/msgstore"
	"node_messager/pkg/node"
)

func newLogger(t *testing.T) *zap.SugaredLogger {
	t.Helper()
	l, _ := zap.NewDevelopment()
	return l.Sugar()
}

// startHub creates a real TCP listener backed by a hub. Returns the node and a
// channel that receives every raw message delivered to the hub's clients.
func startHub(t *testing.T, id int, name string) (node.Node, chan []byte) {
	t.Helper()
	store := msgstore.New(100)
	log := newLogger(t)
	h := hub.New(name, log, store)
	go h.Run()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	// collector: dial the hub so fan-out reaches us
	recv := make(chan []byte, 64)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go h.Serve(conn)
		}
	}()

	// dial a reader connection
	readerConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { readerConn.Close() })
	go func() {
		buf := make([]byte, 65536)
		for {
			n, err := readerConn.Read(buf)
			if err != nil {
				return
			}
			line := make([]byte, n)
			copy(line, buf[:n])
			recv <- line
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	n := node.Node{ID: id, Name: name, Host: "127.0.0.1", Port: port}
	return n, recv
}

func newTestPool(t *testing.T) *Pool {
	t.Helper()
	p := NewPool(newLogger(t))
	t.Cleanup(p.CloseAll)
	return p
}

func readMsg(t *testing.T, ch chan []byte, timeout time.Duration) dto.Message {
	t.Helper()
	select {
	case data := <-ch:
		var m dto.Message
		// strip trailing newline
		for len(data) > 0 && (data[len(data)-1] == '\n' || data[len(data)-1] == '\r') {
			data = data[:len(data)-1]
		}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal message: %v (raw: %q)", err, data)
		}
		return m
	case <-time.After(timeout):
		t.Fatal("timeout waiting for message")
		return dto.Message{}
	}
}

func TestPool_Send_DeliversSingleMessage(t *testing.T) {
	from := node.Node{ID: 99, Name: "sender", Host: "127.0.0.1", Port: 0}
	to, recv := startHub(t, 1, "target")
	pool := newTestPool(t)

	if err := pool.Send(from, to, dto.TypePing, ""); err != nil {
		t.Fatal(err)
	}

	msg := readMsg(t, recv, 2*time.Second)
	if msg.Type != dto.TypePing {
		t.Fatalf("want TypePing, got %q", msg.Type)
	}
}

func TestPool_SendJSON_MarshalsThenSends(t *testing.T) {
	from := node.Node{ID: 99, Name: "sender"}
	to, recv := startHub(t, 1, "target")
	pool := newTestPool(t)

	payload := dto.ProposePayload{RoundID: "round-xyz", Operation: "INSERT_TICKET", Data: "{}"}
	if err := pool.SendJSON(from, to, dto.TypePropose, payload); err != nil {
		t.Fatal(err)
	}

	msg := readMsg(t, recv, 2*time.Second)
	if msg.Type != dto.TypePropose {
		t.Fatalf("want TypePropose, got %q", msg.Type)
	}
	var p dto.ProposePayload
	if err := json.Unmarshal([]byte(msg.Content), &p); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if p.RoundID != "round-xyz" {
		t.Fatalf("want round-xyz, got %q", p.RoundID)
	}
}

func TestPool_BroadcastJSON_SendsToAllTargets(t *testing.T) {
	from := node.Node{ID: 99, Name: "sender"}
	nodeA, recvA := startHub(t, 1, "nodeA")
	nodeB, recvB := startHub(t, 2, "nodeB")
	pool := newTestPool(t)

	errs := pool.BroadcastJSON(from, []node.Node{nodeA, nodeB}, dto.TypePing, struct{}{})
	if len(errs) > 0 {
		t.Fatalf("broadcast errors: %v", errs)
	}

	readMsg(t, recvA, 2*time.Second)
	readMsg(t, recvB, 2*time.Second)
}

func TestPool_Broadcast_ErrorForUnreachableNode(t *testing.T) {
	from := node.Node{ID: 99, Name: "sender"}
	unreachable := node.Node{ID: 1, Name: "dead", Host: "127.0.0.1", Port: 19999}
	pool := newTestPool(t)

	errs := pool.Broadcast(from, []node.Node{unreachable}, dto.TypePing, "")
	if len(errs) == 0 {
		t.Fatal("expected error for unreachable node")
	}
}

func TestPool_Send_ReusesConnection(t *testing.T) {
	from := node.Node{ID: 99, Name: "sender"}
	to, _ := startHub(t, 1, "target")
	pool := newTestPool(t)

	_ = pool.Send(from, to, dto.TypePing, "")
	_ = pool.Send(from, to, dto.TypePing, "")

	pool.mu.Lock()
	n := len(pool.conns)
	pool.mu.Unlock()
	if n != 1 {
		t.Fatalf("want 1 connection in pool, got %d", n)
	}
}

func TestPool_CloseAll_MarksConnectionsClosed(t *testing.T) {
	from := node.Node{ID: 99, Name: "sender"}
	to, _ := startHub(t, 1, "target")
	pool := newTestPool(t)

	_ = pool.Send(from, to, dto.TypePing, "")
	pool.mu.Lock()
	client := pool.conns[to.ID]
	pool.mu.Unlock()

	pool.CloseAll()

	// give readLoop time to detect close
	time.Sleep(50 * time.Millisecond)
	if !client.IsClosed() {
		t.Fatal("connection should be closed after CloseAll")
	}
}
