package main

import (
	"node_messager/internal/adapters/tui"
	logger "node_messager/pkg/logger"
)

func main() {

	globalLog := logger.NewLogger(true, true)

	_, err := tui.NewTui()

	if err != nil {
		globalLog.Fatalf("error initializing cli app: %v", err)
	}

}
