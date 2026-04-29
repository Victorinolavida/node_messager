package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"go.uber.org/zap"
	"node_messager/internal/adapters/cli"
	"node_messager/internal/config"
	logger "node_messager/pkg/logger"
	"node_messager/pkg/msgstore"
	"node_messager/pkg/node"
	tcpserver "node_messager/pkg/tcp_server"
)

// overridden at build time: go build -ldflags "-X main.debug=false" ./cmd
var debug = "true"

func main() {
	startupLog := logger.NewLogger(true, true)

	cfg, err := config.LoadConfig("nodes.json")
	if err != nil {
		startupLog.Fatalf("load config: %v", err)
	}

	if err := os.MkdirAll("logs", 0755); err != nil {
		startupLog.Fatalf("create logs dir: %v", err)
	}
	if err := os.MkdirAll("messages", 0755); err != nil {
		startupLog.Fatalf("create messages dir: %v", err)
	}

	debugMode := debug == "true"

	// When host is defined, only run server+store for host node.
	// Otherwise run server+store for every node in the list.
	serveNodes := cfg.Nodes
	if cfg.HostNode != nil {
		serveNodes = []node.Node{*cfg.HostNode}
	}

	// When host is defined, only the host node's store is file-backed — remote
	// nodes run on other machines so a local file would not reflect their real state.
	// When no host is defined (dev mode), all nodes get file-backed stores.
	allNodes := append([]node.Node{}, cfg.Nodes...)
	if cfg.HostNode != nil {
		allNodes = append(allNodes, *cfg.HostNode)
	}
	stores := make(map[int]*msgstore.Store, len(allNodes))
	for _, n := range allNodes {
		isLocal := cfg.HostNode == nil || n.ID == cfg.HostNode.ID
		if isLocal {
			store, err := msgstore.NewWithFile(50, fmt.Sprintf("messages/%s.jsonl", n.Name))
			if err != nil {
				startupLog.Fatalf("[%s] open message file: %v", n.Name, err)
			}
			stores[n.ID] = store
		} else {
			stores[n.ID] = msgstore.New(50)
		}
	}

	nodeLogs := make(map[int]*zap.SugaredLogger, len(serveNodes))
	var wg sync.WaitGroup
	for _, n := range serveNodes {
		n := n

		f, err := os.OpenFile(fmt.Sprintf("logs/%s.log", n.Name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			startupLog.Fatalf("[%s] open log file: %v", n.Name, err)
		}

		nodeLog := logger.NewLoggerToWriter(f, debugMode)
		nodeLogs[n.ID] = nodeLog
		nodeCtx := logger.SetContextLogger(context.Background(), nodeLog)

		wg.Add(1)
		srv := tcpserver.New(n, stores[n.ID])
		go func() {
			wg.Done()
			if err := srv.Start(nodeCtx); err != nil {
				nodeLog.Errorf("[%s] server error: %s", n.Name, err)
			}
		}()
	}
	wg.Wait()

	if err := cli.Run(cfg.Nodes, stores, cfg.HostNode, startupLog, nodeLogs); err != nil {
		startupLog.Fatalf("cli error: %v", err)
	}
}
