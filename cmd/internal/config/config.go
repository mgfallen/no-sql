package config

import (
	"os"
)

type Config struct {
	AppPort string
}

func Load() *Config {
	port := os.Getenv("APP_PORT")
	if port == "" {
		panic("No APP_PORT environment variable set")
	}

	return &Config{
		AppPort: port,
	}
}
