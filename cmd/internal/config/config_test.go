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
			name: "Success: all required fields present",
			envs: map[string]string{
				"APP_HOST":             "localhost",
				"APP_PORT":             "8080",
				"APP_USER_SESSION_TTL": "60",
				"REDIS_HOST":           "localhost",
				"REDIS_PORT":           "6379",
			},
			expectPanic: false,
		},
		{
			name: "Failure: APP_PORT missing",
			envs: map[string]string{
				"APP_HOST":             "localhost",
				"APP_USER_SESSION_TTL": "60",
				"REDIS_HOST":           "localhost",
				"REDIS_PORT":           "6379",
			},
			expectPanic: true,
		},
		{
			name: "Failure: TTL is not an integer",
			envs: map[string]string{
				"APP_HOST":             "localhost",
				"APP_PORT":             "8080",
				"APP_USER_SESSION_TTL": "invalid-number",
				"REDIS_HOST":           "localhost",
				"REDIS_PORT":           "6379",
			},
			expectPanic: true,
		},
		{
			name: "Success: optional fields provided",
			envs: map[string]string{
				"APP_HOST":             "127.0.0.1",
				"APP_PORT":             "3000",
				"APP_USER_SESSION_TTL": "120",
				"REDIS_HOST":           "redis-prod",
				"REDIS_PORT":           "6380",
				"REDIS_PASSWORD":       "top-secret",
				"REDIS_DB":             "2",
			},
			expectPanic: false,
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
				assert.Equal(t, tt.envs["APP_HOST"], cfg.AppHost)
				assert.Equal(t, tt.envs["APP_PORT"], cfg.AppPort)

				ttl, _ := strconv.Atoi(tt.envs["APP_USER_SESSION_TTL"])
				assert.Equal(t, ttl, cfg.SessionTTL)

				if pwd, ok := tt.envs["REDIS_PASSWORD"]; ok {
					assert.Equal(t, pwd, cfg.RedisPassword)
				}
			}
		})
	}
}
