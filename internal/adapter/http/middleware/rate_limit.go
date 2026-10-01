package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/di-eqa/backend/internal/adapter/http/response"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/ulule/limiter/v3"
	ginlimiter "github.com/ulule/limiter/v3/drivers/middleware/gin"
	redisstore "github.com/ulule/limiter/v3/drivers/store/redis"
)

// RateLimit creates a Redis-backed rate limiting middleware. The format string
// follows ulule/limiter: "<count>-<period>" (S, M, H, D). Limit and store
// errors use the standard error shape.
func RateLimit(redisClient *redis.Client, format string) gin.HandlerFunc {
	store, err := redisstore.NewStoreWithOptions(redisClient, limiter.StoreOptions{Prefix: "rate_limit"})
	if err != nil {
		panic("rate limit: failed to create redis store: " + err.Error())
	}
	rate, err := limiter.NewRateFromFormatted(format)
	if err != nil {
		panic("rate limit: invalid format string: " + err.Error())
	}
	instance := limiter.New(store, rate, limiter.WithTrustForwardHeader(true))

	return ginlimiter.NewMiddleware(instance,
		ginlimiter.WithLimitReachedHandler(func(c *gin.Context) {
			retry := 60
			if reset, err := strconv.ParseInt(c.Writer.Header().Get("X-RateLimit-Reset"), 10, 64); err == nil {
				if d := int(time.Until(time.Unix(reset, 0)).Seconds()) + 1; d > 0 {
					retry = d
				}
			}
			c.Header("Retry-After", strconv.Itoa(retry))
			response.Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "คำขอมากเกินไป กรุณาลองใหม่ภายหลัง", nil)
		}),
		ginlimiter.WithErrorHandler(func(c *gin.Context, _ error) {
			response.Error(c, http.StatusInternalServerError, service.CodeInternal, "เกิดข้อผิดพลาดภายในระบบ", nil)
		}),
	)
}
