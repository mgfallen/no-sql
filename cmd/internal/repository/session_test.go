package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestSessionRepository_CreateSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		sid           string
		ttl           int
		setup         func(mr *miniredis.Miniredis)
		expectedExist bool
		expectedNX    bool
	}{
		{
			name:          "Success: create new session",
			sid:           "fresh-sid",
			ttl:           30,
			setup:         func(mr *miniredis.Miniredis) {},
			expectedExist: true,
			expectedNX:    true,
		},
		{
			name: "Failure: sid already exists (NX check)",
			sid:  "taken-sid",
			ttl:  30,
			setup: func(mr *miniredis.Miniredis) {
				mr.HSet("sid:taken-sid", "created_at", "2026-01-01T00:00:00Z")
			},
			expectedExist: true,
			expectedNX:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			repo := NewSessionRepository(client, tt.ttl)
			ctx := context.Background()

			tt.setup(mr)

			created, err := repo.CreateSession(ctx, tt.sid)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedNX, created)
			assert.Equal(t, tt.expectedExist, mr.Exists("sid:"+tt.sid))

			if created {
				assert.Equal(t, time.Duration(tt.ttl)*time.Second, mr.TTL("sid:"+tt.sid))
				assert.NotEmpty(t, mr.HGet("sid:"+tt.sid, "created_at"))
			}
		})
	}
}

func TestSessionRepository_RefreshTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sid     string
		ttl     int
		prepare func(mr *miniredis.Miniredis)
	}{
		{
			name: "Successfully update TTL and updated_at",
			sid:  "active-session",
			ttl:  60,
			prepare: func(mr *miniredis.Miniredis) {
				key := "sid:active-session"
				pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
				mr.HSet(key, "created_at", pastTime)
				mr.HSet(key, "updated_at", pastTime)
				mr.SetTTL(key, 10*time.Second)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			repo := NewSessionRepository(client, tt.ttl)
			ctx := context.Background()

			tt.prepare(mr)

			oldUpdatedAt := mr.HGet("sid:"+tt.sid, "updated_at")

			err := repo.RefreshTTL(ctx, tt.sid)

			assert.NoError(t, err)
			assert.Equal(t, time.Duration(tt.ttl)*time.Second, mr.TTL("sid:"+tt.sid))
			assert.NotEqual(t, oldUpdatedAt, mr.HGet("sid:"+tt.sid, "updated_at"))
		})
	}
}

func TestSessionRepository_Exists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		sid            string
		setup          func(mr *miniredis.Miniredis)
		expectedResult bool
	}{
		{
			name: "Session exists",
			sid:  "existing-id",
			setup: func(mr *miniredis.Miniredis) {
				mr.HSet("sid:existing-id", "dummy", "data")
			},
			expectedResult: true,
		},
		{
			name:           "Session does not exist",
			sid:            "unknown-id",
			setup:          func(mr *miniredis.Miniredis) {},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			repo := NewSessionRepository(client, 60)
			ctx := context.Background()

			tt.setup(mr)

			exists, err := repo.Exists(ctx, tt.sid)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult, exists)
		})
	}
}
