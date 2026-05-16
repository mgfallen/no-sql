package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var createSessionScript = redis.NewScript(`
    if redis.call("EXISTS", KEYS[1]) == 0 then
        redis.call("HSET", KEYS[1], "created_at", ARGV[1], "updated_at", ARGV[1])
        redis.call("EXPIRE", KEYS[1], ARGV[2])
        return 1
    end
    return 0
`)

// CreateSession атомарно создает сессию через Lua-скрипт, если её еще не было, и проставляет TTL
func (r *Repository) CreateSession(ctx context.Context, sid string) (bool, error) {
	key := fmt.Sprintf("sid:%s", sid)
	now := time.Now().Format(time.RFC3339)

	res, err := createSessionScript.Run(ctx, r.client, []string{key}, now, int(r.ttl.Seconds())).Int64()
	if err != nil {
		return false, fmt.Errorf("failed to execute create session lua script: %w", err)
	}

	return res == 1, nil
}

// RefreshTTL обновляет время последней активности сессии и продлевает её жизнь в Redis
func (r *Repository) RefreshTTL(ctx context.Context, sid string) error {
	key := fmt.Sprintf("sid:%s", sid)
	now := time.Now().Format(time.RFC3339)

	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, "updated_at", now)
	pipe.Expire(ctx, key, r.ttl)

	_, err := pipe.Exec(ctx)
	return err
}

// Exists проверяет, существует ли сессия в базе данных
func (r *Repository) Exists(ctx context.Context, sid string) (bool, error) {
	key := fmt.Sprintf("sid:%s", sid)
	res, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis EXISTS command failed: %w", err)
	}
	return res > 0, nil
}

// DeleteSession удаляет сессию
func (r *Repository) DeleteSession(ctx context.Context, sid string) error {
	return r.client.Del(ctx, fmt.Sprintf("sid:%s", sid)).Err()
}

// BindUser связывает сессию с вошедшим/зарегистрированным пользователем в рамках Hash-структуры сессии
func (r *Repository) BindUser(ctx context.Context, sid string, userID string) error {
	key := fmt.Sprintf("sid:%s", sid)
	now := time.Now().Format(time.RFC3339)

	pipe := r.client.Pipeline()

	pipe.HSet(ctx, key, "user_id", userID)
	pipe.HSet(ctx, key, "updated_at", now)
	pipe.Expire(ctx, key, r.ttl)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to bind user to session in redis: %w", err)
	}
	return nil
}

// GetUserIDBySession возвращает ID пользователя, который привязан к данной сессии
func (r *Repository) GetUserIDBySession(ctx context.Context, sid string) (string, error) {
	key := fmt.Sprintf("sid:%s", sid)

	userID, err := r.client.HGet(ctx, key, "user_id").Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get user_id from redis session: %w", err)
	}

	return userID, nil
}
