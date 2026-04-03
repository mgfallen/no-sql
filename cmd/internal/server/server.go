package server

import (
	"net"
	"net/http"
)

type Server struct {
	httpServer *http.Server
}

// New - конструктор
func New(
	host, port string,
	healthHandler http.HandlerFunc,
	sessionHandler http.Handler,
	registerHandler http.HandlerFunc,
	loginHandler http.HandlerFunc,
	logoutHandler http.HandlerFunc,
	createEventHandler http.HandlerFunc,
	listEventsHandler http.HandlerFunc,
) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", healthHandler)

	mux.Handle("/session", sessionHandler)

	mux.HandleFunc("/users", registerHandler)

	mux.HandleFunc("/auth/login", loginHandler)
	mux.HandleFunc("/auth/logout", logoutHandler)

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			createEventHandler(w, r)
		case http.MethodGet:
			listEventsHandler(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

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
