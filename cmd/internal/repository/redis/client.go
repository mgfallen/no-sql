package redis

import (
	"time"

	"github.com/redis/go-redis/v9"
)

type Repository struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRepository создает новый экземпляр репозитория для работы с Redis
func NewRepository(client *redis.Client, ttlSeconds int) *Repository {
	return &Repository{
		client: client,
		ttl:    time.Duration(ttlSeconds) * time.Second,
	}
}
