package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"node_messager/internal/adapters/tui"
	httpserver "node_messager/pkg/http_server"
	"node_messager/pkg/logbuffer"
	logger "node_messager/pkg/logger"
	"node_messager/pkg/msgstore"
	"node_messager/pkg/node"
)

// overridden at build time: go build -ldflags "-X main.debug=false" ./cmd
var debug = "true"

var nodes = []node.Node{
	{ID: 0, Name: "alpha", Host: "127.0.0.1", Port: 5000},
	{ID: 1, Name: "beta", Host: "127.0.0.1", Port: 5001},
	{ID: 2, Name: "gamma", Host: "127.0.0.1", Port: 5002},
	{ID: 3, Name: "delta", Host: "127.0.0.1", Port: 5003},
}

func main() {
	startupLog := logger.NewLogger(true, true)

	if err := os.MkdirAll("logs", 0755); err != nil {
		startupLog.Fatalf("create logs dir: %v", err)
	}

	debugMode := debug == "true"

	buf := logbuffer.New(500)
	stores := make(map[int]*msgstore.Store, len(nodes))
	for _, n := range nodes {
		stores[n.ID] = msgstore.New(50)
	}

	var wg sync.WaitGroup
	for _, n := range nodes {
		n := n

		f, err := os.OpenFile(fmt.Sprintf("logs/%s.log", n.Name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			startupLog.Fatalf("[%s] open log file: %v", n.Name, err)
		}

		nodeLog := logger.NewLoggerForNode(buf, f, debugMode)
		nodeCtx := logger.SetContextLogger(context.Background(), nodeLog)

		wg.Add(1)
		srv := httpserver.NewHttpServer(n, stores[n.ID])
		go func() {
			wg.Done()
			if err := srv.Start(nodeCtx); err != nil {
				nodeLog.Errorf("[%s] server error: %s", n.Name, err)
			}
		}()
	}
	wg.Wait()

	_, err := tui.NewTui(buf, nodes, stores)
	if err != nil {
		startupLog.Fatalf("error initializing tui: %v", err)
	}
}
