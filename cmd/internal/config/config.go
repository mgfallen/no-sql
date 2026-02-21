package config

import (
	"os"
)

type Config struct {
	AppPort string
	AppHost string
}

func Load() *Config {
	port := os.Getenv("APP_PORT")
	if port == "" {
		panic("No APP_PORT environment variable set")
	}

	host := os.Getenv("APP_HOST")
	if host == "" {
		panic("No APP_HOST environment variable set")
	}

	return &Config{
		AppPort: port,
		AppHost: host,
	}
}
