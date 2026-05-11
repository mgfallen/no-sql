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
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	cfg := config.Load()

	// 1. Исправляем опечатку в названии базы данных из ТЗ (DATABSE)
	mongoDBName := os.Getenv("MONGODB_DATABSE")
	if mongoDBName == "" {
		mongoDBName = cfg.MongoDatabase
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	mongoURI := fmt.Sprintf("mongodb://%s:%s", cfg.MongoHost, cfg.MongoPort)
	clientOptions := options.Client().ApplyURI(mongoURI)

	if cfg.MongoUser != "" {
		clientOptions.SetAuth(options.Credential{
			AuthSource:    "admin",
			Username:      cfg.MongoUser,
			Password:      cfg.MongoPassword,
			AuthMechanism: "SCRAM-SHA-1",
		})
	}

	mClient, err := mongo.Connect(clientOptions)
	if err != nil {
		log.Fatalf("Failed to create Mongo client: %v", err)
	}

	var pingErr error
	for i := 0; i < 15; i++ {
		pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		pingErr = mClient.Ping(pingCtx, nil)
		cancel()
		if pingErr == nil {
			log.Println("Successfully connected to MongoDB!")
			break
		}
		log.Printf("Waiting for MongoDB... (attempt %d/15): %v", i+1, pingErr)
		time.Sleep(2 * time.Second)
	}

	if pingErr != nil {
		log.Fatalf("Failed to connect to MongoDB after retries: %v", pingErr)
	}

	mDB := mClient.Database(mongoDBName)

	sessionRepo := repository.NewSessionRepository(rdb, cfg.SessionTTL)
	mongoRepo := repository.NewMongoRepository(mDB)

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

	// 4. КРИТИЧНО: Чтобы Docker мог прокинуть порт, сервер ДОЛЖЕН слушать "0.0.0.0"
	// В cfg.AppHost у тебя может быть 127.0.0.1, что "запрет" сервер внутри контейнера.
	srv := server.New(
		"0.0.0.0", // Слушаем на всех интерфейсах
		cfg.AppPort,
		healthH.Health,
		sessionH,
		userH.Register,
		userH.Login,
		userH.Logout,
		userH.CreateEvent,
		userH.ListEvents,
	)

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down application...")
		_ = rdb.Close()
		_ = mClient.Disconnect(context.Background())
		os.Exit(0)
	}()

	log.Printf("Application is running on 0.0.0.0:%s", cfg.AppPort)
	if err = srv.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
