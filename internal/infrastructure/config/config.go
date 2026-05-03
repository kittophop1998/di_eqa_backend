package config

import (
	"os"
	"time"
)

type Config struct {
	Port           string
	MongoURI       string
	MongoDB        string
	RedisAddr      string
	RedisPassword  string
	JWTSecret      string
	JWTExpiry      time.Duration
	AllowedOrigin  string
	SupabaseURL    string
	SupabaseBucket string
}

func Load() *Config {
	redisAddr := getEnv("REDIS_URL", getEnv("REDIS_ADDR", "redis://localhost:6379"))
	return &Config{
		Port:           getEnv("PORT", "8080"),
		MongoURI:       getEnv("MONGO_URI", "mongodb://mongo:27017"),
		MongoDB:        getEnv("MONGO_DB", "di_eqa"),
		RedisAddr:      redisAddr,
		RedisPassword:  getEnv("REDIS_PASSWORD", ""),
		JWTSecret:      getEnv("JWT_SECRET", "change-me-in-production-please-this-is-a-dev-secret"),
		JWTExpiry:      12 * time.Hour,
		AllowedOrigin:  getEnv("ALLOWED_ORIGIN", "*"),
		SupabaseURL:    getEnv("SUPABASE_URL", "https://wcrrkhgfaxfcxegotpef.supabase.co"),
		SupabaseBucket: getEnv("SUPABASE_BUCKET", "images_eqa"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
