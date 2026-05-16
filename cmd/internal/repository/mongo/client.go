package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Config struct {
	URI         string
	DBName      string
	MaxPoolSize uint64
	Timeout     time.Duration
}

type Repository struct {
	client     *mongo.Client
	db         *mongo.Database
	collection *mongo.Collection // Добавлено поле для работы с коллекцией событий
}

// NewMongoRepository создает подключение к БД и возвращает готовый репозиторий
func NewMongoRepository(ctx context.Context, cfg Config) (*Repository, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	clientOpts := options.Client().
		ApplyURI(cfg.URI).
		SetMaxPoolSize(cfg.MaxPoolSize)

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mongo: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("failed to ping mongo: %w", err)
	}

	database := client.Database(cfg.DBName)

	return &Repository{
		client:     client,
		db:         database,
		collection: database.Collection("events"), // Инициализируем коллекцию "events"
	}, nil
}

// Close корректно закрывает соединение с MongoDB
func (r *Repository) Close(ctx context.Context) error {
	return r.client.Disconnect(ctx)
}

// GetUnderlyingDB возвращает чистый инстанс *mongo.Database (полезно для тестов)
func (r *Repository) GetUnderlyingDB() *mongo.Database {
	return r.db
}
