package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		appPort     string
		appHost     string
		expectPanic bool
	}{
		{
			name:        "1. Both APP_PORT and APP_HOST are set",
			appPort:     "8080",
			appHost:     "localhost",
			expectPanic: false,
		},
		{
			name:        "2. APP_PORT is not set",
			appPort:     "",
			appHost:     "localhost",
			expectPanic: true,
		},
		{
			name:        "3. APP_HOST is not set",
			appPort:     "8080",
			appHost:     "",
			expectPanic: true,
		},
		{
			name:        "4. Both APP_PORT and APP_HOST are not set",
			appPort:     "",
			appHost:     "",
			expectPanic: true,
		},
		{
			name:        "5. APP_PORT with different value",
			appPort:     "3000",
			appHost:     "127.0.0.1",
			expectPanic: false,
		},
		{
			name:        "6. APP_HOST with different value",
			appPort:     "9090",
			appHost:     "0.0.0.0",
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			oldPort := os.Getenv("APP_PORT")
			oldHost := os.Getenv("APP_HOST")

			assert.NoError(t, os.Setenv("APP_PORT", tt.appPort))
			assert.NoError(t, os.Setenv("APP_HOST", tt.appHost))

			defer func() {
				assert.NoError(t, os.Setenv("APP_PORT", oldPort))
				assert.NoError(t, os.Setenv("APP_HOST", oldHost))
			}()

			if tt.expectPanic {
				assert.Panics(t, func() {
					Load()
				})
				return
			}

			cfg := Load()
			assert.Equal(t, tt.appPort, cfg.AppPort)
			assert.Equal(t, tt.appHost, cfg.AppHost)
		})
	}
}
