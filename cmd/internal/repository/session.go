package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type SessionRepository struct {
	client *redis.Client
	ttl    time.Duration
}

// NewSessionRepository - конструктор
func NewSessionRepository(client *redis.Client, ttlSeconds int) *SessionRepository {
	return &SessionRepository{
		client: client,
		ttl:    time.Duration(ttlSeconds) * time.Second,
	}
}

func (r *SessionRepository) CreateSession(ctx context.Context, sid string) (bool, error) {
	key := fmt.Sprintf("sid:%s", sid)
	now := time.Now().Format(time.RFC3339)

	pipe := r.client.TxPipeline()

	nxRes := pipe.HSetNX(ctx, key, "created_at", now)
	pipe.HSet(ctx, key, "updated_at", now)
	pipe.Expire(ctx, key, r.ttl)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	return nxRes.Val(), nil
}

func (r *SessionRepository) RefreshTTL(ctx context.Context, sid string) error {
	key := fmt.Sprintf("sid:%s", sid)
	now := time.Now().Format(time.RFC3339)

	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, "updated_at", now)
	pipe.Expire(ctx, key, r.ttl)

	_, err := pipe.Exec(ctx)
	return err
}

func (r *SessionRepository) Exists(ctx context.Context, sid string) (bool, error) {
	key := fmt.Sprintf("sid:%s", sid)
	res, err := r.client.Exists(ctx, key).Result()
	if err != nil && errors.Is(err, redis.Nil) {
		return false, nil
	}
	return res > 0, err
}
