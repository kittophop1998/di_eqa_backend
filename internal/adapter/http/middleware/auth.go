package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/di-eqa/backend/internal/adapter/http/response"
	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/gin-gonic/gin"
)

// PrincipalKey is the gin context key holding the *service.Principal.
const PrincipalKey = "principal"

// TokenVerifier is implemented by the security adapter.
type TokenVerifier interface {
	Verify(string) (applicationport.TokenClaims, error)
}

// Authenticator resolves a token subject to a database-backed principal.
type Authenticator interface {
	Authenticate(ctx context.Context, userID string) (*service.Principal, error)
}

// Auth validates the Bearer token, then loads the user from the database on
// every request, so role, status and hospital always come from the DB and a
// disabled or demoted account is rejected immediately (BR-02). Tokens are
// accepted from the Authorization header only, never from the query string (A-03).
func Auth(tokens TokenVerifier, authn Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			response.Error(c, http.StatusUnauthorized, service.CodeUnauthenticated, "กรุณาเข้าสู่ระบบ", nil)
			return
		}
		claims, err := tokens.Verify(token)
		if err != nil {
			if errors.Is(err, applicationport.ErrTokenExpired) {
				response.Error(c, http.StatusUnauthorized, service.CodeTokenExpired, "เซสชันหมดอายุ กรุณาเข้าสู่ระบบใหม่", nil)
				return
			}
			response.Error(c, http.StatusUnauthorized, service.CodeUnauthenticated, "กรุณาเข้าสู่ระบบ", nil)
			return
		}
		p, err := authn.Authenticate(c.Request.Context(), claims.UserID)
		if err != nil {
			response.FromError(c, err)
			return
		}
		c.Set(PrincipalKey, p)
		c.Next()
	}
}

func bearerToken(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// PrincipalFrom returns the authenticated principal set by Auth, or nil.
func PrincipalFrom(c *gin.Context) *service.Principal {
	v, ok := c.Get(PrincipalKey)
	if !ok {
		return nil
	}
	p, _ := v.(*service.Principal)
	return p
}

func forbid(c *gin.Context) {
	response.Error(c, http.StatusForbidden, service.CodeForbidden, "คุณไม่มีสิทธิ์เข้าถึงส่วนนี้", nil)
}

// RequireStaff allows admin and super_admin only (BR-03). It is applied once
// to the whole /api/admin group, so no handler needs its own role check.
func RequireStaff() gin.HandlerFunc {
	return func(c *gin.Context) {
		p := PrincipalFrom(c)
		if p == nil || !p.Role.IsStaff() {
			forbid(c)
			return
		}
		c.Next()
	}
}

// RequireSuperAdmin allows super_admin only.
func RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		p := PrincipalFrom(c)
		if p == nil || !p.Role.IsSuper() {
			forbid(c)
			return
		}
		c.Next()
	}
}
