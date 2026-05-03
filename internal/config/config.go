package config

import (
	"os"
	"time"
)

type Config struct {
	Port          string
	MongoURI      string
	MongoDB       string
	RedisAddr     string
	RedisPassword string
	JWTSecret     string
	JWTExpiry     time.Duration
	AllowedOrigin string
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		MongoURI:      getEnv("MONGO_URI", "mongodb://mongo:27017"),
		MongoDB:       getEnv("MONGO_DB", "di_eqa"),
		RedisAddr:     getEnv("REDIS_ADDR", "redis:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		JWTSecret:     getEnv("JWT_SECRET", "change-me-in-production-please-this-is-a-dev-secret"),
		JWTExpiry:     12 * time.Hour,
		AllowedOrigin: getEnv("ALLOWED_ORIGIN", "*"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
