package hub

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"node_messager/pkg/dto"
	"node_messager/pkg/msgstore"
)

func newTestHub(t *testing.T) (*Hub, *msgstore.Store, string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "hub-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	f.Close()

	store, err := msgstore.NewWithFile(100, path)
	if err != nil {
		t.Fatal(err)
	}

	log, _ := zap.NewDevelopment()
	h := New("test", log.Sugar(), store)
	go h.Run()
	return h, store, path
}

func connectWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

func sendJSON(t *testing.T, conn *websocket.Conn, m dto.Message) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func waitForFileEntries(t *testing.T, path string, want int, timeout time.Duration) []msgstore.Entry {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entries := readEntries(t, path)
		if len(entries) >= want {
			return entries
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d entries in file", want)
	return nil
}

func readEntries(t *testing.T, path string) []msgstore.Entry {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []msgstore.Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e msgstore.Entry
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		entries = append(entries, e)
	}
	return entries
}

func TestHub_BroadcastMessageSavedToFile(t *testing.T) {
	h, _, path := newTestHub(t)
	srv := httptest.NewServer(http.HandlerFunc(h.ServeWs))
	defer srv.Close()

	conn := connectWS(t, srv)
	defer conn.Close()

	m := dto.Message{
		ID:       "bc-1",
		Type:     "broadcast",
		FromNode: "nodeA",
		ToNode:   "",
		Content:  "hello all",
	}
	sendJSON(t, conn, m)

	entries := waitForFileEntries(t, path, 1, 2*time.Second)
	if entries[0].Msg.ID != m.ID {
		t.Errorf("want id %q got %q", m.ID, entries[0].Msg.ID)
	}
	if entries[0].Type != msgstore.Received {
		t.Errorf("want type %q got %q", msgstore.Received, entries[0].Type)
	}
}

func TestHub_DirectMessageSavedToFile(t *testing.T) {
	h, _, path := newTestHub(t)
	srv := httptest.NewServer(http.HandlerFunc(h.ServeWs))
	defer srv.Close()

	conn := connectWS(t, srv)
	defer conn.Close()

	m := dto.Message{
		ID:       "dm-1",
		Type:     "direct",
		FromNode: "nodeA",
		ToNode:   "nodeB",
		Content:  "private message",
	}
	sendJSON(t, conn, m)

	entries := waitForFileEntries(t, path, 1, 2*time.Second)
	if entries[0].Msg.ID != m.ID {
		t.Errorf("want id %q got %q", m.ID, entries[0].Msg.ID)
	}
	if entries[0].Msg.ToNode != m.ToNode {
		t.Errorf("want to_node %q got %q", m.ToNode, entries[0].Msg.ToNode)
	}
}

func TestHub_MultipleMixedMessagesSavedToFile(t *testing.T) {
	h, _, path := newTestHub(t)
	srv := httptest.NewServer(http.HandlerFunc(h.ServeWs))
	defer srv.Close()

	conn := connectWS(t, srv)
	defer conn.Close()

	messages := []dto.Message{
		{ID: "1", Type: "broadcast", FromNode: "nodeA", Content: "hi all"},
		{ID: "2", Type: "direct", FromNode: "nodeA", ToNode: "nodeB", Content: "hey B"},
		{ID: "3", Type: "broadcast", FromNode: "nodeB", Content: "yo"},
	}
	for _, m := range messages {
		sendJSON(t, conn, m)
	}

	entries := waitForFileEntries(t, path, len(messages), 2*time.Second)
	ids := make(map[string]bool)
	for _, e := range entries {
		ids[e.Msg.ID] = true
	}
	for _, m := range messages {
		if !ids[m.ID] {
			t.Errorf("message id %q not found in file", m.ID)
		}
	}
}
