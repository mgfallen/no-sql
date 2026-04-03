package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AppHost string
	AppPort string

	SessionTTL int

	// REDIS
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int

	// MONGODB
	MongoDatabase string
	MongoUser     string
	MongoPassword string
	MongoHost     string
	MongoPort     string
}

func Load() *Config {
	// Сначала проверяем базу с опечаткой, потом нормальную
	mongoDb := os.Getenv("MONGODB_DATABSE")
	if mongoDb == "" {
		mongoDb = getEnvString("MONGODB_DATABASE", "eventhub")
	}

	return &Config{
		AppHost: getEnvString("APP_HOST", "0.0.0.0"),
		AppPort: getEnvString("APP_PORT", "8080"),

		SessionTTL: requiredEnvInt("APP_USER_SESSION_TTL"),

		RedisHost:     requiredEnv("REDIS_HOST"),
		RedisPort:     requiredEnv("REDIS_PORT"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		MongoDatabase: mongoDb,
		MongoUser:     os.Getenv("MONGODB_USER"),
		MongoPassword: os.Getenv("MONGODB_PASSWORD"),
		MongoHost:     requiredEnv("MONGODB_HOST"),
		MongoPort:     requiredEnv("MONGODB_PORT"),
	}
}

func requiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("Environment variable %s is required but not set", key))
	}
	return value
}

func requiredEnvInt(key string) int {
	valueStr := requiredEnv(key)
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		panic(fmt.Sprintf("Environment variable %s must be an integer, got: %s", key, valueStr))
	}
	return value
}

func getEnvInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvString(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
