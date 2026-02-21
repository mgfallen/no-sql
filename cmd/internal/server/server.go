package server

import (
	"fmt"
	"net/http"
)

type Server struct {
	httpServer *http.Server
}

func New(host, port string, healthHandler http.HandlerFunc) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	addr := fmt.Sprintf("%s:%s", host, port)

	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
}

func (s *Server) Run() error {
	return s.httpServer.ListenAndServe()
}
