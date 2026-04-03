package config

import (
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

var envMu sync.Mutex

func TestLoad_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		envs        map[string]string
		expectPanic bool
	}{
		{
			name: "Success: all required fields present (Redis + Mongo)",
			envs: map[string]string{
				"APP_HOST":             "localhost",
				"APP_PORT":             "8080",
				"APP_USER_SESSION_TTL": "60",
				"REDIS_HOST":           "localhost",
				"REDIS_PORT":           "6379",
				"MONGODB_DATABASE":     "eventhub",
				"MONGODB_HOST":         "mongo",
				"MONGODB_PORT":         "27017",
			},
			expectPanic: false,
		},
		{
			name: "Success: handle typo in MONGODB_DATABSE",
			envs: map[string]string{
				"APP_HOST":             "localhost",
				"APP_PORT":             "8080",
				"APP_USER_SESSION_TTL": "60",
				"REDIS_HOST":           "localhost",
				"REDIS_PORT":           "6379",
				"MONGODB_DATABSE":      "typo_db",
				"MONGODB_HOST":         "mongo",
				"MONGODB_PORT":         "27017",
			},
			expectPanic: false,
		},
		{
			name: "Failure: required REDIS_HOST missing",
			envs: map[string]string{
				"APP_HOST":             "localhost",
				"APP_PORT":             "8080",
				"APP_USER_SESSION_TTL": "60",
				"MONGODB_DATABASE":     "eventhub",
				"MONGODB_HOST":         "mongo",
				"MONGODB_PORT":         "27017",
			},
			expectPanic: true,
		},
		{
			name: "Failure: MONGODB_PORT missing",
			envs: map[string]string{
				"APP_HOST":             "localhost",
				"APP_PORT":             "8080",
				"APP_USER_SESSION_TTL": "60",
				"REDIS_HOST":           "localhost",
				"REDIS_PORT":           "6379",
				"MONGODB_HOST":         "mongo",
			},
			expectPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			envMu.Lock()
			defer envMu.Unlock()

			oldEnvs := make(map[string]string)
			keysToTest := []string{
				"APP_HOST", "APP_PORT", "APP_USER_SESSION_TTL",
				"REDIS_HOST", "REDIS_PORT", "REDIS_PASSWORD", "REDIS_DB",
				"MONGODB_DATABASE", "MONGODB_DATABSE", "MONGODB_HOST", "MONGODB_PORT", "MONGODB_USER", "MONGODB_PASSWORD",
			}

			for _, k := range keysToTest {
				oldEnvs[k] = os.Getenv(k)
				os.Unsetenv(k)
			}

			defer func() {
				for k, v := range oldEnvs {
					if v != "" {
						os.Setenv(k, v)
					} else {
						os.Unsetenv(k)
					}
				}
			}()

			for k, v := range tt.envs {
				os.Setenv(k, v)
			}

			if tt.expectPanic {
				assert.Panics(t, func() {
					Load()
				})
			} else {
				cfg := Load()

				if val, ok := tt.envs["APP_HOST"]; ok {
					assert.Equal(t, val, cfg.AppHost)
				}

				if val, ok := tt.envs["MONGODB_DATABSE"]; ok {
					assert.Equal(t, val, cfg.MongoDatabase)
				} else if val, ok := tt.envs["MONGODB_DATABASE"]; ok {
					assert.Equal(t, val, cfg.MongoDatabase)
				}

				ttl, _ := strconv.Atoi(tt.envs["APP_USER_SESSION_TTL"])
				assert.Equal(t, ttl, cfg.SessionTTL)
			}
		})
	}
}
