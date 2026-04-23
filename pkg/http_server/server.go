package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"node_messager/pkg/logger"
	"time"
)

type httpServer struct {
	srv *http.Server
	mux *http.ServeMux
}

var (
	defaultHost = "localhost"
	defaultPort = 8080
)

func NewHttpServer(host string, port int) *httpServer {
	addr := formattAddress(host, port)
	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: 10 * time.Second,
		IdleTimeout: 100 * time.Second,
	}

	return &httpServer{srv, mux}
}

func (s *httpServer) AddRoute(method, path string, handler http.HandlerFunc) {
	pattern := fmt.Sprintf("%s %s", method, path)
	s.mux.HandleFunc(pattern, handler)
}

func formattAddress(host string, port int) string {
	if port == 0 {
		port = defaultPort
	}
	if host == "" {
		host = defaultHost
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func (s *httpServer) Start(ctx context.Context) error {
	l := logger.GetContextLogger(ctx)
	l.Infof("start server on %s", s.srv.Addr)
	return s.srv.ListenAndServe()
}
