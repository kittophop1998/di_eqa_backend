package cache

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Connect creates and validates a Redis client.
// Supports full URL (redis://...) and addr+password forms.
func Connect(addr, password string) (*redis.Client, error) {
	var opts *redis.Options

	if isRedisURL(addr) {
		parsed, err := redis.ParseURL(addr)
		if err != nil {
			return nil, err
		}
		opts = parsed
	} else {
		opts = &redis.Options{
			Addr:     addr,
			Password: password,
			DB:       0,
		}
	}

	opts.DialTimeout = 10 * time.Second
	opts.ReadTimeout = 5 * time.Second
	opts.WriteTimeout = 5 * time.Second

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	log.Printf("✅ Redis connected: %s", addr)
	return rdb, nil
}

func isRedisURL(s string) bool {
	return strings.HasPrefix(s, "redis://") || strings.HasPrefix(s, "rediss://")
}
