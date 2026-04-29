package tcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"node_messager/pkg/dto"
	"node_messager/pkg/msgstore"
	"node_messager/pkg/node"
)

func newTestServer(t *testing.T) (*tcpServer, net.Listener) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	store := msgstore.New(100)
	n := node.Node{Name: "test-node", Port: 0}
	srv := New(n, store)
	return srv, ln
}

func TestStart_PortInUse(t *testing.T) {
	// bind all interfaces so server.Start (which also binds 0.0.0.0) gets EADDRINUSE
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	store := msgstore.New(100)
	srv := New(node.Node{Name: "clash", Port: port}, store)

	// Start returns immediately with an error — no context cancel needed
	err = srv.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for port already in use, got nil")
	}
}

func TestServe_ClientConnects(t *testing.T) {
	srv, ln := newTestServer(t)
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = srv.serve(ctx, ln) }()

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
}

func TestServe_ContextCancelShutsDown(t *testing.T) {
	srv, ln := newTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- srv.serve(ctx, ln) }()

	// give server time to start accepting
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned error after cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not shut down within 2s after context cancel")
	}
}

func TestServe_MultipleClients(t *testing.T) {
	srv, ln := newTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = srv.serve(ctx, ln) }()

	const n = 3
	conns := make([]net.Conn, n)
	for i := range conns {
		c, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
		if err != nil {
			t.Fatalf("dial client %d: %v", i, err)
		}
		conns[i] = c
	}
	for _, c := range conns {
		c.Close()
	}
}

func TestServe_MessageBroadcast(t *testing.T) {
	srv, ln := newTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = srv.serve(ctx, ln) }()

	addr := ln.Addr().String()

	sender, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial sender: %v", err)
	}
	defer sender.Close()

	receiver, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial receiver: %v", err)
	}
	defer receiver.Close()

	// give hub time to register both clients
	time.Sleep(50 * time.Millisecond)

	msg := dto.Message{
		ID:       "test-id-1",
		Type:     "chat",
		FromNode: "a",
		ToNode:   "b",
		Content:  "hello",
	}
	data, _ := json.Marshal(msg)
	if _, err := sender.Write(append(data, '\n')); err != nil {
		t.Fatalf("write: %v", err)
	}

	_ = receiver.SetReadDeadline(time.Now().Add(2 * time.Second))
	scanner := bufio.NewScanner(receiver)
	if !scanner.Scan() {
		t.Fatalf("expected broadcast line, got nothing: %v", scanner.Err())
	}

	var got dto.Message
	if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != msg.ID || got.Content != msg.Content {
		t.Fatalf("got %+v, want %+v", got, msg)
	}
}
