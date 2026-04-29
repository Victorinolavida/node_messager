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
	for {
		raw := prompt(sc, "choice> ")
		if raw == "" {
			return node.Node{}, false // EOF or cancel
		}
		for i, n := range nodes {
			if raw == fmt.Sprintf("%d", i+1) || strings.EqualFold(raw, n.Name) {
				return n, true
			}
		}
		fmt.Println("invalid — enter number or node name")
	}
}

func sendMsg(pool *connPool, from, to node.Node, content string, stores map[int]*msgstore.Store) error {
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
	// do NOT save Received here — the hub on the receiving node owns that write
	return nil
}

func broadcast(pool *connPool, from node.Node, nodes []node.Node, content string, stores map[int]*msgstore.Store) []string {
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
		// do NOT save Received here — the hub on the receiving node owns that write
	}
	return errs
}

func printEntries(nodeName string, entries []msgstore.Entry) {
	if len(entries) == 0 {
		fmt.Printf("no messages for %s yet\n", nodeName)
		return
	}
	for _, e := range entries {
		fmt.Printf("%s  %-10s  %-10s  from=%-8s  to=%-8s  %q\n",
			e.At.Format(time.RFC3339),
			e.Type,
			e.Msg.Type,
			e.Msg.FromNode,
			e.Msg.ToNode,
			e.Msg.Content,
		)
	}
}

// ── main loop ─────────────────────────────────────────────────────────────────

func Run(nodes []node.Node, stores map[int]*msgstore.Store, hostNode *node.Node, log *zap.SugaredLogger) error {
	pool := newConnPool(log)
	defer pool.closeAll()

	sc := bufio.NewScanner(os.Stdin)

	for {
		fmt.Println()
		fmt.Println("1) send message")
		fmt.Println("2) broadcast")
		fmt.Println("3) messages per node")
		fmt.Println("4) list nodes")
		fmt.Println("5) quit")

		choice := prompt(sc, "> ")
		switch choice {

		case "1", "send":
			var from node.Node
			var ok bool
			if hostNode != nil {
				from = *hostNode
			} else {
				if from, ok = pickNode(sc, nodes, "from node:"); !ok {
					continue
				}
			}
			targets := make([]node.Node, 0, len(nodes)-1)
			for _, n := range nodes {
				if n.ID != from.ID {
					targets = append(targets, n)
				}
			}
			to, ok := pickNode(sc, targets, "to node:")
			if !ok {
				continue
			}
			content := prompt(sc, "message: ")
			if content == "" {
				fmt.Println("message cannot be empty")
				continue
			}
			if err := sendMsg(pool, from, to, content, stores); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println("sent")
			}

		case "2", "broadcast":
			var from node.Node
			var ok bool
			if hostNode != nil {
				from = *hostNode
			} else {
				if from, ok = pickNode(sc, nodes, "from node:"); !ok {
					continue
				}
			}
			content := prompt(sc, "message: ")
			if content == "" {
				fmt.Println("message cannot be empty")
				continue
			}
			if errs := broadcast(pool, from, nodes, content, stores); len(errs) > 0 {
				for _, e := range errs {
					fmt.Printf("error: %s\n", e)
				}
			} else {
				fmt.Println("broadcast sent")
			}

		case "3", "messages":
			n, ok := pickNode(sc, nodes, "messages for node:")
			if !ok {
				continue
			}
			entries, _ := stores[n.ID].Latest(50)
			printEntries(n.Name, entries)

		case "4", "list":
			for _, n := range nodes {
				fmt.Printf("  %-8s  %s:%d\n", n.Name, n.Host, n.Port)
			}

		case "5", "q", "quit", "exit", "":
			return nil

		default:
			fmt.Println("unknown command")
		}
	}
}
