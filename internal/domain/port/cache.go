package port

import (
	"context"
	"time"
)

// ZMember represents a sorted set member with its score.
type ZMember struct {
	Member string
	Score  float64
}

// CachePort — port for key-value cache operations (driven).
type CachePort interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
	Delete(ctx context.Context, key string) error
	ZAdd(ctx context.Context, key string, score float64, member string) error
	ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]ZMember, error)
	HSet(ctx context.Context, key, field string, value interface{}) error
	HGet(ctx context.Context, key, field string) (string, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
}
