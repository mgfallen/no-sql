package main

import (
	"log"
	"no-sql/cmd/internal/config"
	"no-sql/cmd/internal/handler"
	"no-sql/cmd/internal/server"
)

func main() {
	cfg := config.Load()

	healthHandler := handler.NewHealthHandler()

	srv := server.New(
		cfg.AppPort,
		healthHandler.Health,
	)

	log.Printf("Starting server on port %s...\n", cfg.AppPort)

	if err := srv.Run(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
