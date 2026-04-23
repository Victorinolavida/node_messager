package main

import (
	"context"
	"node_messager/internal/adapters/tui"
	httpserver "node_messager/pkg/http_server"
	"node_messager/pkg/logbuffer"
	logger "node_messager/pkg/logger"
	"sync"
)

func main() {
	startupLog := logger.NewLogger(true, true)

	buf := logbuffer.New(500)
	bufLog := logger.NewLoggerToWriter(buf, true)
	ctxWithLog := logger.SetContextLogger(context.Background(), bufLog)

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		srv := httpserver.NewHttpServer("", 3333+i)
		go func() {
			wg.Done()
			if err := srv.Start(ctxWithLog); err != nil {
				bufLog.Errorf("server error: %s", err)
			}
		}()
	}
	wg.Wait()

	_, err := tui.NewTui(buf)
	if err != nil {
		startupLog.Fatalf("error initializing tui: %v", err)
	}
}
