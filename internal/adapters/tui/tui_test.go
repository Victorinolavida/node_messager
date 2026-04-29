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

func TestFormatEntries_ReceivedAppears(t *testing.T) {
	store := msgstore.New(100)
	saveMsg(t, store, "bc-1", "broadcast", "nodeA", "", "hello all", msgstore.Received)

	entries, err := store.Latest(50)
	if err != nil {
		t.Fatal(err)
	}

	result := formatEntries("nodeA", entries)

	if !strings.Contains(result, "hello all") {
		t.Errorf("received message missing from output: %q", result)
	}
	if !strings.Contains(result, string(msgstore.Received)) {
		t.Errorf("entry type %q not in output: %q", msgstore.Received, result)
	}
}

func TestFormatEntries_SentAppears(t *testing.T) {
	store := msgstore.New(100)
	saveMsg(t, store, "dm-1", "direct", "nodeA", "nodeB", "private", msgstore.Sent)

	entries, err := store.Latest(50)
	if err != nil {
		t.Fatal(err)
	}

	result := formatEntries("nodeA", entries)
	if !strings.Contains(result, "private") {
		t.Errorf("sent message missing from output: %q", result)
	}
}

func TestFormatEntries_MixedMessages_AllPresent(t *testing.T) {
	store := msgstore.New(100)
	saveMsg(t, store, "1", "broadcast", "nodeA", "", "hi all", msgstore.Received)
	saveMsg(t, store, "2", "direct", "nodeA", "nodeB", "hey B", msgstore.Sent)
	saveMsg(t, store, "3", "broadcast", "nodeC", "", "yo", msgstore.Received)

	entries, err := store.Latest(50)
	if err != nil {
		t.Fatal(err)
	}

	result := formatEntries("nodeA", entries)
	for _, content := range []string{"hi all", "hey B", "yo"} {
		if !strings.Contains(result, content) {
			t.Errorf("content %q missing from output: %q", content, result)
		}
	}
}

func TestFormatEntries_EmptyStore_ReturnsNoMessagesText(t *testing.T) {
	store := msgstore.New(100)
	entries, _ := store.Latest(50)

	result := formatEntries("nodeA", entries)

	if !strings.Contains(result, "No messages for nodeA yet.") {
		t.Errorf("want no-messages text, got: %q", result)
	}
}

func TestFormatEntries_NewNode_FileCreated_ShowsNoMessages(t *testing.T) {
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

	result := formatEntries("nodeX", entries)

	if !strings.Contains(result, "No messages for nodeX yet.") {
		t.Errorf("want no-messages text for new node, got: %q", result)
	}
}

func TestFormatEntries_ShowsBothSentAndReceived(t *testing.T) {
	path := fmt.Sprintf("%s/host-node.jsonl", t.TempDir())

	store, err := msgstore.NewWithFile(50, path)
	if err != nil {
		t.Fatal(err)
	}

	saveMsg(t, store, "s-1", "direct", "hostNode", "nodeB", "sent msg", msgstore.Sent)
	saveMsg(t, store, "r-1", "broadcast", "nodeB", "", "received msg", msgstore.Received)

	entries, _ := store.Latest(50)
	result := formatEntries("hostNode", entries)

	if !strings.Contains(result, "sent msg") {
		t.Errorf("sent message missing from output: %q", result)
	}
	if !strings.Contains(result, "received msg") {
		t.Errorf("received message missing from output: %q", result)
	}
}

func TestFormatEntries_TimestampPresent(t *testing.T) {
	store := msgstore.New(100)
	saveMsg(t, store, "ts-1", "direct", "nodeA", "nodeB", "timed msg", msgstore.Sent)

	entries, _ := store.Latest(50)
	result := formatEntries("nodeA", entries)

	year := time.Now().UTC().Format("2006")
	if !strings.Contains(result, year) {
		t.Errorf("expected timestamp year %s in output: %q", year, result)
	}
}
