package cache

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Connect เชื่อมต่อ Redis รองรับ 2 รูปแบบ:
//  1. URL เต็ม เช่น redis://default:password@host:port  (Railway / Upstash / ฯลฯ)
//  2. addr + password แยก เช่น "redis:6379" + "secret"
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
	log.Printf("✅ Redis connected: %s", opts.Addr)
	return rdb, nil
}

// isRedisURL ตรวจว่า string เป็น redis:// หรือ rediss:// URL
func isRedisURL(s string) bool {
	return len(s) > 8 && (s[:8] == "redis://" || (len(s) > 9 && s[:9] == "rediss://"))
}
