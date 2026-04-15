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
	getEventHandler http.HandlerFunc,
	updateEventHandler http.HandlerFunc,
	listUsersHandler http.HandlerFunc,
) *Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", healthHandler)
	mux.Handle("/session", sessionHandler)

	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			listUsersHandler(w, r)
		} else if r.Method == http.MethodPost {
			registerHandler(w, r)
		}
	})

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

	mux.HandleFunc("/events/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getEventHandler(w, r)
		} else if r.Method == http.MethodPatch {
			updateEventHandler(w, r)
		} else {
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
