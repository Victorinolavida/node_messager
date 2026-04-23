package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"node_messager/pkg/hub"
	"node_messager/pkg/logger"
	"node_messager/pkg/msgstore"
	"node_messager/pkg/node"
	"time"
)

type httpServer struct {
	node  node.Node
	srv   *http.Server
	mux   *http.ServeMux
	store *msgstore.Store
}

func NewHttpServer(n node.Node, store *msgstore.Store) *httpServer {
	addr := fmt.Sprintf("%s:%d", n.Host, n.Port)
	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: 10 * time.Second,
		IdleTimeout: 100 * time.Second,
	}

	return &httpServer{node: n, srv: srv, mux: mux, store: store}
}

func (s *httpServer) AddRoute(method, path string, handler http.HandlerFunc) {
	pattern := fmt.Sprintf("%s %s", method, path)
	s.mux.HandleFunc(pattern, handler)
}

func (s *httpServer) Start(ctx context.Context) error {
	l := logger.GetContextLogger(ctx)
	h := hub.New(s.node.Name, l, s.store)
	s.mux.HandleFunc("/ws", h.ServeWs)
	go h.Run()
	l.Infof("[%s] listening on %s", s.node.Name, s.srv.Addr)
	return s.srv.ListenAndServe()
}
