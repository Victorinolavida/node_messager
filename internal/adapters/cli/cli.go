package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"node_messager/pkg/dto"
	"node_messager/pkg/msgstore"
	"node_messager/pkg/node"
	"node_messager/pkg/tcpclient"
)

const logDir = "logs"
const logLines = 50

// ── connection pool ───────────────────────────────────────────────────────────

type connPool struct {
	mu    sync.Mutex
	conns map[int]*tcpclient.Client
	log   *zap.SugaredLogger
}

func newConnPool(log *zap.SugaredLogger) *connPool {
	return &connPool{conns: make(map[int]*tcpclient.Client), log: log}
}

func (p *connPool) closeAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, c := range p.conns {
		if err := c.Close(); err != nil {
			p.log.Debugf("[pool] close error node_id=%d: %v", id, err)
		}
		delete(p.conns, id)
	}
}

func (p *connPool) get(n node.Node) (*tcpclient.Client, error) {
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

// ── helpers ───────────────────────────────────────────────────────────────────

func separator() {
	fmt.Println()
}

func prompt(sc *bufio.Scanner, label string) string {
	fmt.Print(label)
	if !sc.Scan() {
		return ""
	}
	return strings.TrimSpace(sc.Text())
}

func pickNode(sc *bufio.Scanner, nodes []node.Node, label string) (node.Node, bool) {
	fmt.Println(label)
	for i, n := range nodes {
		fmt.Printf("  %d) %-8s  %s:%d\n", i+1, n.Name, n.Host, n.Port)
	}
	fmt.Println()
	for {
		raw := prompt(sc, "choice> ")
		if raw == "" {
			return node.Node{}, false
		}
		for i, n := range nodes {
			if raw == fmt.Sprintf("%d", i+1) || strings.EqualFold(raw, n.Name) {
				return n, true
			}
		}
		fmt.Println("invalid — enter number or node name")
	}
}

func sendMsg(pool *connPool, from, to node.Node, content string, stores map[int]*msgstore.Store, log *zap.SugaredLogger) error {
	c, err := pool.get(to)
	if err != nil {
		return err
	}
	m := dto.Message{
		ID:       uuid.New().String(),
		Type:     dto.TypeMsg,
		FromNode: from.Name,
		ToNode:   to.Name,
		Content:  content,
		SendAt:   time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := c.Send(data); err != nil {
		return err
	}
	if s, ok := stores[from.ID]; ok {
		_ = s.Save(m, msgstore.Sent)
	}
	log.Infof("[%s] sent  type=%s to=%s id=%s — %q", from.Name, m.Type, to.Name, m.ID, content)
	// do NOT save Received here — the hub on the receiving node owns that write
	return nil
}

func broadcast(pool *connPool, from node.Node, nodes []node.Node, content string, stores map[int]*msgstore.Store, log *zap.SugaredLogger) []string {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	var errs []string
	for _, n := range nodes {
		c, err := pool.get(n)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", n.Name, err))
			continue
		}
		m := dto.Message{
			ID:       id,
			Type:     dto.TypeBroadcast,
			FromNode: from.Name,
			ToNode:   n.Name,
			Content:  content,
			SendAt:   now,
		}
		data, _ := json.Marshal(m)
		if err := c.Send(data); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", n.Name, err))
			continue
		}
		if s, ok := stores[from.ID]; ok {
			_ = s.Save(m, msgstore.Sent)
		}
		log.Infof("[%s] sent  type=%s to=%s id=%s — %q", from.Name, m.Type, n.Name, id, content)
		// do NOT save Received here — the hub on the receiving node owns that write
	}
	return errs
}

func tailLogFile(nodeName string, n int) {
	path := fmt.Sprintf("%s/%s.log", logDir, nodeName)
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("cannot open log file %s: %v\n", path, err)
		return
	}
	defer f.Close()

	// collect all lines, keep last n
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) == 0 {
		fmt.Printf("no logs for %s yet\n", nodeName)
		return
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for _, l := range lines {
		fmt.Println(l)
	}
}

func formatEntries(nodeName string, entries []msgstore.Entry) string {
	if len(entries) == 0 {
		return fmt.Sprintf("No messages for %s yet.", nodeName)
	}
	var sb strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&sb, "%s  %-10s  %-10s  from=%-8s  to=%-8s  %q\n",
			e.At.Format(time.RFC3339),
			e.Type,
			e.Msg.Type,
			e.Msg.FromNode,
			e.Msg.ToNode,
			e.Msg.Content,
		)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func printEntries(nodeName string, entries []msgstore.Entry) {
	fmt.Println(formatEntries(nodeName, entries))
}

// nodeLog returns the file-backed logger for nodeID when available, else fallback.
func nodeLog(nodeLogs map[int]*zap.SugaredLogger, nodeID int, fallback *zap.SugaredLogger) *zap.SugaredLogger {
	if l, ok := nodeLogs[nodeID]; ok {
		return l
	}
	return fallback
}

// ── main loop ─────────────────────────────────────────────────────────────────

func Run(nodes []node.Node, stores map[int]*msgstore.Store, hostNode *node.Node, log *zap.SugaredLogger, nodeLogs map[int]*zap.SugaredLogger) error {
	pool := newConnPool(log)
	defer pool.closeAll()

	sc := bufio.NewScanner(os.Stdin)

	for {
		separator()
		fmt.Println("  node messager")
		fmt.Println()
		fmt.Println("  1) send message")
		fmt.Println("  2) broadcast")
		fmt.Println("  3) messages per node")
		fmt.Println("  4) logs per node")
		fmt.Println("  5) list nodes")
		fmt.Println("  6) quit")
		separator()

		choice := prompt(sc, "> ")
		switch choice {

		case "1", "send":
			separator()
			fmt.Println("  send message")
			separator()
			var from node.Node
			var ok bool
			if hostNode != nil {
				from = *hostNode
				fmt.Printf("  from: %s\n\n", from.Name)
			} else {
				if from, ok = pickNode(sc, nodes, "  from node:"); !ok {
					continue
				}
			}
			targets := make([]node.Node, 0, len(nodes)-1)
			for _, n := range nodes {
				if n.ID != from.ID {
					targets = append(targets, n)
				}
			}
			to, ok := pickNode(sc, targets, "  to node:")
			if !ok {
				continue
			}
			fmt.Printf("\n  %s → %s\n", from.Name, to.Name)
			content := prompt(sc, "  message: ")
			if content == "" {
				fmt.Println("\n  error: message cannot be empty")
				continue
			}
			separator()
			if err := sendMsg(pool, from, to, content, stores, nodeLog(nodeLogs, from.ID, log)); err != nil {
				fmt.Printf("  error: %v\n", err)
			} else {
				fmt.Println("  ✓ sent")
			}

		case "2", "broadcast":
			separator()
			fmt.Println("  broadcast")
			separator()
			var from node.Node
			var ok bool
			if hostNode != nil {
				from = *hostNode
				fmt.Printf("  from: %s → all nodes\n\n", from.Name)
			} else {
				if from, ok = pickNode(sc, nodes, "  from node:"); !ok {
					continue
				}
				fmt.Printf("\n  %s → all nodes\n", from.Name)
			}
			content := prompt(sc, "  message: ")
			if content == "" {
				fmt.Println("\n  error: message cannot be empty")
				continue
			}
			separator()
			targets := make([]node.Node, 0, len(nodes)-1)
			for _, n := range nodes {
				if n.ID != from.ID {
					targets = append(targets, n)
				}
			}
			if errs := broadcast(pool, from, targets, content, stores, nodeLog(nodeLogs, from.ID, log)); len(errs) > 0 {
				for _, e := range errs {
					fmt.Printf("  error: %s\n", e)
				}
			} else {
				fmt.Println("  ✓ broadcast sent")
			}

		case "3", "messages":
			separator()
			fmt.Println("  messages per node")
			separator()
			if hostNode != nil {
				fmt.Printf("  messages — %s\n\n", hostNode.Name)
				entries, _ := stores[hostNode.ID].Latest(50)
				printEntries(hostNode.Name, entries)
			} else {
				n, ok := pickNode(sc, nodes, "  select node:")
				if !ok {
					continue
				}
				separator()
				fmt.Printf("  messages — %s\n\n", n.Name)
				entries, _ := stores[n.ID].Latest(50)
				printEntries(n.Name, entries)
			}

		case "4", "logs":
			separator()
			fmt.Println("  logs per node")
			separator()
			if hostNode != nil {
				fmt.Printf("  logs — %s\n\n", hostNode.Name)
				tailLogFile(hostNode.Name, logLines)
			} else {
				n, ok := pickNode(sc, nodes, "  select node:")
				if !ok {
					continue
				}
				separator()
				fmt.Printf("  logs — %s\n\n", n.Name)
				tailLogFile(n.Name, logLines)
			}

		case "5", "list":
			separator()
			fmt.Println("  nodes")
			separator()
			for _, n := range nodes {
				fmt.Printf("  %-8s  %s:%d\n", n.Name, n.Host, n.Port)
			}

		case "6", "q", "quit", "exit", "":
			return nil

		default:
			fmt.Println("  unknown command")
		}
	}
}
