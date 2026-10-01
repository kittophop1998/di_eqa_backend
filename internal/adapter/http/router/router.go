// Package router wires every HTTP route to its handler.
package router

import (
	"net/http"
	"strings"

	"github.com/di-eqa/backend/internal/adapter/http/handler"
	"github.com/di-eqa/backend/internal/adapter/http/middleware"
	"github.com/di-eqa/backend/internal/adapter/http/response"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Deps bundles everything the router needs.
type Deps struct {
	// AllowedOrigins is the exact CORS allow-list; empty means no CORS headers
	// (same-origin only). "*" is never used (A-03).
	AllowedOrigins []string
	// Redis backs rate limiting. When nil, rate limiting is skipped (tests).
	Redis *redis.Client

	TokenVerifier middleware.TokenVerifier
	Authenticator middleware.Authenticator

	AuthHandler     *handler.AuthHandler
	HospitalHandler *handler.HospitalHandler
	MeHandler       *handler.MeHandler
	AdminHandler    *handler.AdminHandler
	AuditHandler    *handler.AuditHandler
}

// Setup registers all middleware and routes on r.
//
// The legacy live-session surface (/quizzes, /submissions, /sessions,
// /leaderboard, /ws, /hospitals, /users, /audit-logs) is intentionally NOT
// registered: it trusted client input and JWT-embedded roles. Epic G deletes
// the code; until then nothing routes to it.
func Setup(r *gin.Engine, d Deps) *gin.Engine {
	r.Use(middleware.RequestID(), middleware.Recovery(), middleware.AccessLog())

	if len(d.AllowedOrigins) > 0 {
		r.Use(cors.New(cors.Config{
			AllowOrigins:     d.AllowedOrigins,
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept", response.RequestIDHeader},
			ExposeHeaders:    []string{"Content-Length", response.RequestIDHeader, "Retry-After"},
			AllowCredentials: false,
		}))
	}

	limiter := func(format string) gin.HandlerFunc {
		if d.Redis == nil {
			return func(c *gin.Context) { c.Next() }
		}
		return middleware.RateLimit(d.Redis, format)
	}
	authLimiter := limiter("10-M")
	apiLimiter := limiter("120-M")

	auth := middleware.Auth(d.TokenVerifier, d.Authenticator)
	staff := middleware.RequireStaff()

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Public
	pub := r.Group("/api", apiLimiter)
	pub.GET("/public/hospitals", d.HospitalHandler.PublicList)
	pub.POST("/auth/register", authLimiter, d.AuthHandler.Register)
	pub.POST("/auth/login", authLimiter, d.AuthHandler.Login)

	// Authenticated (any active user)
	authed := r.Group("/api", apiLimiter, auth)
	authed.GET("/auth/me", d.AuthHandler.Me)

	me := authed.Group("/me")
	me.GET("/quizzes", d.MeHandler.ListQuizzes)
	me.GET("/quizzes/:quizId", d.MeHandler.GetQuiz)
	me.POST("/quizzes/:quizId/attempts", d.MeHandler.StartAttempt)
	me.GET("/attempts", d.MeHandler.ListAttempts)
	me.GET("/attempts/:attemptId", d.MeHandler.GetAttempt)
	me.PUT("/attempts/:attemptId/answers", d.MeHandler.SaveAnswers)
	me.POST("/attempts/:attemptId/submit", d.MeHandler.Submit)
	me.GET("/attempts/:attemptId/result", d.MeHandler.Result)
	me.GET("/attempts/:attemptId/questions/:questionId/image", d.MeHandler.Image)
	me.GET("/certificates", d.MeHandler.ListCertificates)
	me.GET("/certificates/:id", d.MeHandler.GetCertificate)

	// Admin: one group, one role check (BR-01, BR-03). Role `user` gets 403.
	admin := authed.Group("/admin", staff)
	admin.GET("/hospitals", d.HospitalHandler.AdminList)
	admin.POST("/hospitals", d.HospitalHandler.Create)
	admin.PUT("/hospitals/:id", d.HospitalHandler.Update)
	admin.DELETE("/hospitals/:id", d.HospitalHandler.Delete)

	admin.GET("/users", d.AdminHandler.ListUsers)
	admin.POST("/users/:id/approve", d.AdminHandler.ApproveUser)
	admin.POST("/users/:id/reject", d.AdminHandler.RejectUser)
	admin.PATCH("/users/:id", d.AdminHandler.PatchUser)

	admin.GET("/cell-types", d.AdminHandler.ListCellTypes)
	admin.GET("/cell-images", d.AdminHandler.ListCellImages)

	admin.GET("/quizzes", d.AdminHandler.ListQuizzes)
	admin.POST("/quizzes", d.AdminHandler.CreateQuiz)
	admin.GET("/quizzes/:id", d.AdminHandler.GetQuiz)
	admin.PUT("/quizzes/:id", d.AdminHandler.UpdateQuiz)
	admin.DELETE("/quizzes/:id", d.AdminHandler.DeleteQuiz)
	admin.PUT("/quizzes/:id/hospitals", d.AdminHandler.SetQuizHospitals)
	admin.POST("/quizzes/:id/publish", d.AdminHandler.PublishQuiz)
	admin.POST("/quizzes/:id/archive", d.AdminHandler.ArchiveQuiz)
	admin.POST("/quizzes/:id/hospitals/:hospitalId/open", d.AdminHandler.OpenAssignment)
	admin.POST("/quizzes/:id/hospitals/:hospitalId/close", d.AdminHandler.CloseAssignment)

	// super_admin only
	super := middleware.RequireSuperAdmin()
	admin.PATCH("/users/:id/role", super, d.AdminHandler.SetUserRole)
	admin.GET("/audit-logs", super, d.AuditHandler.List)

	// Unknown paths. Anything under /api/admin/ is authenticated and
	// role-checked first, so a `user` gets 403 (not a probe-able 404).
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/admin/") {
			if auth(c); c.IsAborted() {
				return
			}
			if staff(c); c.IsAborted() {
				return
			}
		}
		response.Error(c, http.StatusNotFound, "NOT_FOUND", "ไม่พบข้อมูลที่ต้องการ", nil)
	})
	return r
}
