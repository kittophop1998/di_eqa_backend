package cache

import (
	"context"
	"time"

	"github.com/di-eqa/backend/internal/domain/port"
	"github.com/redis/go-redis/v9"
)

// RedisCache implements port.CachePort using Redis.
type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

func (r *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	return r.client.SetNX(ctx, key, value, ttl).Result()
}

func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *RedisCache) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return r.client.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err()
}

func (r *RedisCache) ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]port.ZMember, error) {
	zs, err := r.client.ZRevRangeWithScores(ctx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}
	out := make([]port.ZMember, len(zs))
	for i, z := range zs {
		out[i] = port.ZMember{Member: z.Member.(string), Score: z.Score}
	}
	return out, nil
}

func (r *RedisCache) HSet(ctx context.Context, key, field string, value interface{}) error {
	return r.client.HSet(ctx, key, field, value).Err()
}

func (r *RedisCache) HGet(ctx context.Context, key, field string) (string, error) {
	return r.client.HGet(ctx, key, field).Result()
}

func (r *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return r.client.Expire(ctx, key, ttl).Err()
}
