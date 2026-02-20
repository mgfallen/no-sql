package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		envValue    string
		expectPanic bool
	}{
		{
			name:        "1. APP_PORT is set",
			envValue:    "8080",
			expectPanic: false,
		},
		{
			name:        "2. APP_PORT is not set",
			envValue:    "",
			expectPanic: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			t.Setenv("APP_PORT", tt.envValue)

			if tt.expectPanic {
				assert.Panics(t, func() {
					Load()
				})
				return
			}

			cfg := Load()
			assert.Equal(t, tt.envValue, cfg.AppPort)
		})
	}
}
