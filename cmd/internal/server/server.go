package server

import (
	"fmt"
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
	userRegisterHandler http.HandlerFunc,
	userProfileHandler http.HandlerFunc,
) *Server {
	mux := http.NewServeMux()

	// Существующие хендлеры
	mux.HandleFunc("/health", healthHandler)
	mux.Handle("/session", sessionHandler)

	// Новые хендлеры для работы с пользователями
	mux.HandleFunc("/user/register", userRegisterHandler)
	mux.HandleFunc("/user/profile", userProfileHandler)

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
