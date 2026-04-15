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
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	cfg := config.Load()

	// 1. Подключение к Redis (Sessions)
	rdb := redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	// 2. Подключение к MongoDB
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

	// Цикл ожидания доступности БД (Wait-for-it)
	if err := retryPing(mClient); err != nil {
		log.Fatalf("MongoDB unreachable: %v", err)
	}

	// Исправляем опечатку из ТЗ, если переменная передана
	dbName := os.Getenv("MONGODB_DATABSE")
	if dbName == "" {
		dbName = cfg.MongoDatabase
	}
	mDB := mClient.Database(dbName)

	// 3. Инициализация слоев
	sessionRepo := repository.NewSessionRepository(rdb, cfg.SessionTTL)

	// Наш единый репозиторий реализует и UserRepo, и EventRepo
	mongoRepo := repository.NewMongoRepository(mDB)

	// Создаем индексы (Unique для username и title)
	if err = mongoRepo.InitIndices(context.Background()); err != nil {
		log.Printf("Warning: Indices init error (might exist): %v", err)
	}

	// Опционально: Настройка шардирования (для ЛР4)
	enableSharding(context.Background(), mClient, dbName)

	sessionSvc := service.NewSessionService(sessionRepo)
	userSvc := service.NewUserService(mongoRepo, mongoRepo)

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
		userH.GetEvent,
		userH.UpdateEvent,
		userH.ListUsers,
	)

	// 5. Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Application is running on 0.0.0.0:%s", cfg.AppPort)
		if err := srv.Run(); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = rdb.Close()
	_ = mClient.Disconnect(shutdownCtx)

	log.Println("Stopped.")
}

// retryPing проверяет коннект с ретраями
func retryPing(client *mongo.Client) error {
	var err error
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = client.Ping(ctx, nil)
		cancel()
		if err == nil {
			log.Println("Successfully connected to MongoDB!")
			return nil
		}
		log.Printf("Retrying MongoDB connection... (%d/10)", i+1)
		time.Sleep(2 * time.Second)
	}
	return err
}

// enableSharding настраивает шардирование коллекции
func enableSharding(ctx context.Context, client *mongo.Client, dbName string) {
	res := client.Database("admin").RunCommand(ctx, bson.D{{Key: "enableSharding", Value: dbName}})
	if res.Err() != nil {
		log.Printf("Sharding DB notice: %v", res.Err())
	}

	res = client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "shardCollection", Value: fmt.Sprintf("%s.events", dbName)},
		{Key: "key", Value: bson.D{{Key: "created_by", Value: "hashed"}}},
	})
	if res.Err() != nil {
		log.Printf("Sharding Collection notice: %v", res.Err())
	}
}
