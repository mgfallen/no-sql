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
	"no-sql/cmd/internal/repository"
	"no-sql/cmd/internal/server"
	"no-sql/cmd/internal/service"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	cfg := config.Load()

	rdb := redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	mongoCtx, mongoCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer mongoCancel()

	mongoURI := fmt.Sprintf("mongodb://%s:%s", cfg.MongoHost, cfg.MongoPort)
	mClient, err := mongo.Connect(mongoCtx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Failed to create Mongo client: %v", err)
	}

	if err = mClient.Ping(mongoCtx, nil); err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	mDB := mClient.Database(cfg.MongoDatabase)

	sessionRepo := repository.NewSessionRepository(rdb, cfg.SessionTTL)
	userRepo := repository.NewMongoRepository(mDB)

	if err = userRepo.InitIndices(context.Background()); err != nil {
		log.Fatalf("Failed to init MongoDB indices: %v", err)
	}

	sessionSvc := service.NewSessionService(sessionRepo)

	healthH := handler.NewHealthHandler(sessionSvc, cfg.SessionTTL)
	sessionH := handler.NewSessionHandler(sessionSvc, cfg.SessionTTL)
	userH := handler.NewUserHandler(userRepo)

	srv := server.New(
		cfg.AppHost,
		cfg.AppPort,
		healthH.Health,
		sessionH,
		userH.Register,
		userH.GetProfile,
	)

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down application...")

		if err = rdb.Close(); err != nil {
			log.Printf("Error closing Redis: %v", err)
		}
		if err = mClient.Disconnect(context.Background()); err != nil {
			log.Printf("Error closing MongoDB: %v", err)
		}

		os.Exit(0)
	}()

	log.Printf("Application is running on %s:%s", cfg.AppHost, cfg.AppPort)
	log.Printf("Connected to Redis at %s and MongoDB at %s", cfg.RedisHost, cfg.MongoHost)

	if err := srv.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
