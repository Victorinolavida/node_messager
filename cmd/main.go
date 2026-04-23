package main

import (
	"context"
	"node_messager/internal/adapters/tui"
	httpserver "node_messager/pkg/http_server"
	"node_messager/pkg/logbuffer"
	logger "node_messager/pkg/logger"
	"node_messager/pkg/node"
	"sync"
)

var nodes = []node.Node{
	{ID: 0, Name: "alpha", Host: "127.0.0.1", Port: 5000},
	{ID: 1, Name: "beta", Host: "127.0.0.1", Port: 5001},
	{ID: 2, Name: "gamma", Host: "127.0.0.1", Port: 5002},
	{ID: 3, Name: "delta", Host: "127.0.0.1", Port: 5003},
}

func main() {
	startupLog := logger.NewLogger(true, true)

	buf := logbuffer.New(500)
	bufLog := logger.NewLoggerToWriter(buf, true)
	ctxWithLog := logger.SetContextLogger(context.Background(), bufLog)

	var wg sync.WaitGroup
	for _, n := range nodes {
		wg.Add(1)
		srv := httpserver.NewHttpServer(n)
		go func() {
			wg.Done()
			if err := srv.Start(ctxWithLog); err != nil {
				bufLog.Errorf("[%s] server error: %s", n.Name, err)
			}
		}()
	}
	wg.Wait()

	_, err := tui.NewTui(buf, nodes)
	if err != nil {
		startupLog.Fatalf("error initializing tui: %v", err)
	}
}
