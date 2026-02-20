package server

import (
	"net/http"
)

type Server struct {
	httpServer *http.Server
}

func New(port string, healthHandler http.HandlerFunc) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	return &Server{
		httpServer: &http.Server{
			Addr:    ":" + port,
			Handler: mux,
		},
	}
}

func (s *Server) Run() error {
	return s.httpServer.ListenAndServe()
}
