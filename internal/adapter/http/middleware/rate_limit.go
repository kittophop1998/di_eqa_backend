package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/ulule/limiter/v3"
	ginlimiter "github.com/ulule/limiter/v3/drivers/middleware/gin"
	redisstore "github.com/ulule/limiter/v3/drivers/store/redis"
)

// RateLimit creates a Redis-backed rate limiting middleware.
// The format string follows ulule/limiter convention: "<count>-<period>"
// where period is S (second), M (minute), H (hour), D (day).
// Example: "10-M" = 10 requests per minute per IP.
func RateLimit(redisClient *redis.Client, format string) gin.HandlerFunc {
	store, err := redisstore.NewStoreWithOptions(redisClient, limiter.StoreOptions{
		Prefix: "rate_limit",
	})
	if err != nil {
		panic("rate limit: failed to create redis store: " + err.Error())
	}

	rate, err := limiter.NewRateFromFormatted(format)
	if err != nil {
		panic("rate limit: invalid format string: " + err.Error())
	}

	instance := limiter.New(store, rate, limiter.WithTrustForwardHeader(true))

	middleware := ginlimiter.NewMiddleware(instance,
		ginlimiter.WithLimitReachedHandler(func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "too many requests",
				"message": "rate limit exceeded, please slow down",
			})
		}),
	)

	return middleware
}
