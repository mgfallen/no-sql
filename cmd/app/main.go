package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"no-sql/cmd/internal/config"
	"no-sql/cmd/internal/handler"
	"no-sql/cmd/internal/repository/mongo"
	"no-sql/cmd/internal/repository/redis"
	"no-sql/cmd/internal/server"
	"no-sql/cmd/internal/service"

	redisdriver "github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()

	rdb := redisdriver.NewClient(&redisdriver.Options{
		Addr:     net.JoinHostPort(cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	sessionRepo := redis.NewRepository(rdb, cfg.SessionTTL)

	mongoURI := fmt.Sprintf("mongodb://%s:%s", cfg.MongoHost, cfg.MongoPort)
	if cfg.MongoUser != "" {
		mongoURI = fmt.Sprintf("mongodb://%s:%s@%s:%s/?authSource=admin&authMechanism=SCRAM-SHA-1",
			cfg.MongoUser, cfg.MongoPassword, cfg.MongoHost, cfg.MongoPort)
	}

	mongoCfg := mongo.Config{
		URI:         mongoURI,
		DBName:      cfg.MongoDatabase,
		MaxPoolSize: 100,
		Timeout:     5 * time.Second,
	}

	mongoRepo, err := mongo.NewMongoRepository(context.Background(), mongoCfg)
	if err != nil {
		log.Fatalf("Failed to initialize MongoDB repository: %v", err)
	}

	if err = mongoRepo.InitIndices(context.Background()); err != nil {
		log.Fatalf("Failed to init MongoDB indices: %v", err)
	}

	sessionSvc := service.NewSessionService(sessionRepo)
	userSvc := service.NewUserService(mongoRepo)

	healthH := handler.NewHealthHandler(sessionSvc, cfg.SessionTTL)
	sessionH := handler.NewSessionHandler(sessionSvc, cfg.SessionTTL)

	userH := handler.NewUserHandler(
		userSvc,
		sessionSvc,
		sessionRepo,
		cfg.SessionTTL,
	)

	srv := server.New(
		cfg.AppHost,
		cfg.AppPort,
		healthH.Health,
		sessionH,
		userH.Register,
		userH.Login,
		userH.Logout,
		userH.CreateEvent,
		userH.ListEvents,
		userH.UpdateEvent,
	)

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down application...")

		_ = rdb.Close()
		_ = mongoRepo.Close(context.Background())
		os.Exit(0)
	}()

	log.Printf("Application is running on %s:%s", cfg.AppHost, cfg.AppPort)
	if err = srv.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
