package server

import (
	"net"
	"net/http"
)

type Server struct {
	httpServer *http.Server
}

// New - конструктор, использующий встроенный роутинг Go 1.22+ с явным разделением HTTP-методов
func New(
	host, port string,
	healthHandler http.HandlerFunc,
	sessionHandler http.Handler,
	registerHandler http.HandlerFunc,
	loginHandler http.HandlerFunc,
	logoutHandler http.HandlerFunc,
	createEventHandler http.HandlerFunc,
	listEventsHandler http.HandlerFunc,
	updateEventHandler http.HandlerFunc,
) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", healthHandler)
	mux.Handle("/session", sessionHandler)
	mux.HandleFunc("/users", registerHandler)
	mux.HandleFunc("/auth/login", loginHandler)
	mux.HandleFunc("/auth/logout", logoutHandler)

	mux.HandleFunc("POST /events", createEventHandler)
	mux.HandleFunc("GET /events", listEventsHandler)
	mux.HandleFunc("PATCH /events", updateEventHandler)
	mux.HandleFunc("PATCH /events/{id}", updateEventHandler)

	addr := net.JoinHostPort(host, port)

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
