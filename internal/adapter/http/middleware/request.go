package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"regexp"
	"runtime/debug"
	"time"

	"github.com/di-eqa/backend/internal/adapter/http/response"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

// RequestID assigns every request an id, echoes it in X-Request-Id and stores
// it in the context so error bodies and logs can carry it (A-06).
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(response.RequestIDHeader)
		if !safeRequestID.MatchString(id) {
			var b [12]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		c.Set(response.RequestIDKey, id)
		c.Header(response.RequestIDHeader, id)
		c.Next()
	}
}

// Recovery turns a panic into the standard INTERNAL error body.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("request_id=%s panic: %v\n%s", response.RequestID(c), r, debug.Stack())
				response.Error(c, http.StatusInternalServerError, service.CodeInternal, "เกิดข้อผิดพลาดภายในระบบ", nil)
			}
		}()
		c.Next()
	}
}

// AccessLog logs one line per request. It logs the path only, never the query
// string, so tokens or other secrets can't leak into logs.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("request_id=%s %s %s status=%d dur=%s",
			response.RequestID(c), c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start).Round(time.Millisecond))
	}
}
