package middleware

import (
	"net/http"
	"strings"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/gin-gonic/gin"
)

// Auth validates the JWT token from Authorization header or query param.
// TokenVerifier is implemented by the security adapter. Keeping this small
// interface local to the HTTP adapter avoids exposing JWT details to routing.
type TokenVerifier interface {
	Verify(string) (applicationport.TokenClaims, error)
}

func Auth(tokens TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		var token string
		if header != "" {
			parts := strings.SplitN(header, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				token = parts[1]
			}
		}
		if token == "" {
			token = c.Query("token")
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		claims, err := tokens.Verify(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set("userId", claims.UserID)
		c.Set("hospitalId", claims.HospitalID)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// RequireRole ensures the authenticated user has one of the given roles.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		r, _ := role.(string)
		for _, allowed := range roles {
			if r == allowed {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient role"})
	}
}
