package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"node_messager/pkg/dto"
	"node_messager/pkg/msgstore"
)

func saveMsg(t *testing.T, store *msgstore.Store, id, typ, from, to, content string, et msgstore.EntryType) {
	t.Helper()
	m := dto.Message{ID: id, Type: typ, FromNode: from, ToNode: to, Content: content}
	if err := store.Save(m, et); err != nil {
		t.Fatalf("save %s: %v", id, err)
	}
}

func TestViewLogs_BroadcastReceived_AppearsWhenNotSentOnly(t *testing.T) {
	store := msgstore.New(100)
	saveMsg(t, store, "bc-1", "broadcast", "nodeA", "", "hello all", msgstore.Received)

	entries, err := store.Latest(50)
	if err != nil {
		t.Fatal(err)
	}

	result := formatEntries("nodeA", entries, false)

	if !strings.Contains(result, "bc-1") && !strings.Contains(result, "hello all") {
		t.Errorf("broadcast message not in log output: %q", result)
	}
	if !strings.Contains(result, string(msgstore.Received)) {
		t.Errorf("entry type %q not in output: %q", msgstore.Received, result)
	}
}

func TestViewLogs_SentDirect_AppearsInBothViews(t *testing.T) {
	store := msgstore.New(100)
	saveMsg(t, store, "dm-1", "direct", "nodeA", "nodeB", "private", msgstore.Sent)

	entries, err := store.Latest(50)
	if err != nil {
		t.Fatal(err)
	}

	allResult := formatEntries("nodeA", entries, false)
	if !strings.Contains(allResult, "private") {
		t.Errorf("sent direct not in all-view: %q", allResult)
	}

	sentResult := formatEntries("nodeA", entries, true)
	if !strings.Contains(sentResult, "private") {
		t.Errorf("sent direct not in sent-only view: %q", sentResult)
	}
}

func TestViewLogs_ReceivedFiltered_WhenSentOnly(t *testing.T) {
	store := msgstore.New(100)
	saveMsg(t, store, "r-1", "broadcast", "nodeB", "", "incoming", msgstore.Received)
	saveMsg(t, store, "s-1", "direct", "nodeA", "nodeB", "outgoing", msgstore.Sent)

	entries, err := store.Latest(50)
	if err != nil {
		t.Fatal(err)
	}

	result := formatEntries("nodeA", entries, true)

	if strings.Contains(result, "incoming") {
		t.Errorf("received message should be hidden in sent-only view: %q", result)
	}
	if !strings.Contains(result, "outgoing") {
		t.Errorf("sent message should appear in sent-only view: %q", result)
	}
}

func TestViewLogs_EmptyStore_ReturnsNoMessagesText(t *testing.T) {
	store := msgstore.New(100)
	entries, _ := store.Latest(50)

	result := formatEntries("nodeA", entries, false)

	if !strings.Contains(result, "No messages for nodeA yet.") {
		t.Errorf("want no-messages text, got: %q", result)
	}
}

func TestViewLogs_MixedMessages_AllPresentInOutput(t *testing.T) {
	store := msgstore.New(100)
	saveMsg(t, store, "1", "broadcast", "nodeA", "", "hi all", msgstore.Received)
	saveMsg(t, store, "2", "direct", "nodeA", "nodeB", "hey B", msgstore.Sent)
	saveMsg(t, store, "3", "broadcast", "nodeC", "", "yo", msgstore.Received)

	entries, err := store.Latest(50)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries from store, got %d", len(entries))
	}

	result := formatEntries("nodeA", entries, false)

	for _, content := range []string{"hi all", "hey B", "yo"} {
		if !strings.Contains(result, content) {
			t.Errorf("content %q missing from log view: %q", content, result)
		}
	}
}

func TestViewLogs_NewNode_FileCreated_ShowsNoMessages(t *testing.T) {
	path := fmt.Sprintf("%s/new-node.jsonl", t.TempDir())

	store, err := msgstore.NewWithFile(50, path)
	if err != nil {
		t.Fatalf("NewWithFile: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("store file not created on disk")
	}

	entries, err := store.Latest(50)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}

	result := formatEntries("nodeX", entries, false)

	if !strings.Contains(result, "No messages for nodeX yet.") {
		t.Errorf("want no-messages text for new node, got: %q", result)
	}
}

func TestViewLogs_HostNode_ShowsBothSentAndReceived(t *testing.T) {
	path := fmt.Sprintf("%s/host-node.jsonl", t.TempDir())

	store, err := msgstore.NewWithFile(50, path)
	if err != nil {
		t.Fatal(err)
	}

	saveMsg(t, store, "s-1", "direct", "hostNode", "nodeB", "sent msg", msgstore.Sent)
	saveMsg(t, store, "r-1", "broadcast", "nodeB", "", "received msg", msgstore.Received)

	entries, _ := store.Latest(50)
	// host view uses sentOnly=false — must show both
	result := formatEntries("hostNode", entries, false)

	if !strings.Contains(result, "sent msg") {
		t.Errorf("sent message missing from host log view: %q", result)
	}
	if !strings.Contains(result, "received msg") {
		t.Errorf("received message missing from host log view: %q", result)
	}
}

func TestViewLogs_EntryTimestamp_PresentInOutput(t *testing.T) {
	store := msgstore.New(100)
	saveMsg(t, store, "ts-1", "direct", "nodeA", "nodeB", "timed msg", msgstore.Sent)

	entries, _ := store.Latest(50)
	result := formatEntries("nodeA", entries, false)

	year := time.Now().UTC().Format("2006")
	if !strings.Contains(result, year) {
		t.Errorf("expected timestamp year %s in output: %q", year, result)
	}
}
