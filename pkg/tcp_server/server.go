package tcpserver

import (
	"context"
	"fmt"
	"net"

	"node_messager/pkg/hub"
	"node_messager/pkg/logger"
	"node_messager/pkg/msgstore"
	"node_messager/pkg/node"
)

type tcpServer struct {
	node  node.Node
	store *msgstore.Store
}

func New(n node.Node, store *msgstore.Store) *tcpServer {
	return &tcpServer{node: n, store: store}
}

func (s *tcpServer) Start(ctx context.Context) error {
	l := logger.GetContextLogger(ctx)
	addr := fmt.Sprintf(":%d", s.node.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	h := hub.New(s.node.Name, l, s.store)
	go h.Run()
	l.Infof("[%s] listening on %s", s.node.Name, addr)

	go func() {
		<-ctx.Done()
		if err := ln.Close(); err != nil {
			l.Debugf("[%s] listener close error: %v", s.node.Name, err)
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				l.Errorf("[%s] accept error: %v", s.node.Name, err)
				continue
			}
		}
		go h.Serve(conn)
	}
}
