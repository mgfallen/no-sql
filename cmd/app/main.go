package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"no-sql/cmd/internal/config"
	"no-sql/cmd/internal/handler"
	"no-sql/cmd/internal/repository"
	"no-sql/cmd/internal/server"
	"no-sql/cmd/internal/service"

	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()

	// redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	repo := repository.NewSessionRepository(rdb, cfg.SessionTTL)
	svc := service.NewSessionService(repo)

	healthH := handler.NewHealthHandler(svc, cfg.SessionTTL)
	sessionH := handler.NewSessionHandler(svc, cfg.SessionTTL)

	srv := server.New(
		cfg.AppHost,
		cfg.AppPort,
		healthH.Health,
		sessionH,
	)

	// graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down application...")

		if err := rdb.Close(); err != nil {
			log.Printf("Error closing Redis: %v", err)
		}

		os.Exit(0)
	}()

	log.Printf("Application is running on %s:%s", cfg.AppHost, cfg.AppPort)

	if err := srv.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
