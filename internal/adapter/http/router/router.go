package router

// Package router wires every HTTP route to its handler.

import (
	"net/http"

	"github.com/di-eqa/backend/internal/adapter/http/handler"
	"github.com/di-eqa/backend/internal/adapter/http/middleware"
	"github.com/di-eqa/backend/internal/infrastructure/config"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Deps bundles all HTTP handlers the router needs.
type Deps struct {
	Cfg            *config.Config
	HospHandler    *handler.HospitalHandler
	AuthHandler    *handler.AuthHandler
	QuizHandler    *handler.QuizHandler
	SessionHandler *handler.SessionHandler
	WSHandler      *handler.WSHandler
	UserHandler    *handler.UserHandler
	AuditHandler   *handler.AuditHandler
}

// Setup registers all middleware and route groups on the provided *gin.Engine.
func Setup(r *gin.Engine, d Deps) *gin.Engine {
	// CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
	}))

	// Health check
	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "di-eqa-backend"})
	})

	// Public routes
	public := r.Group("/api")
	public.GET("/hospitals", d.HospHandler.List)
	public.GET("/hospitals/:code", d.HospHandler.GetByCode)
	public.POST("/auth/register", d.AuthHandler.Register)
	public.POST("/auth/login", d.AuthHandler.Login)

	// Authenticated routes
	authed := r.Group("/api")
	authed.Use(middleware.Auth(d.Cfg.JWTSecret))

	authed.GET("/auth/me", d.AuthHandler.Me)
	authed.GET("/quizzes", d.QuizHandler.List)
	authed.GET("/quizzes/:id", d.QuizHandler.Get)
	authed.POST("/quizzes/:id/submit", d.QuizHandler.Submit)
	authed.GET("/submissions/me", d.QuizHandler.MyHistory)
	authed.GET("/submissions/:id", d.QuizHandler.GetSubmission)
	authed.GET("/sessions/active", d.SessionHandler.ListActive)
	authed.GET("/sessions/:id", d.SessionHandler.Get)
	authed.GET("/sessions/code/:code", d.SessionHandler.GetByCode)
	authed.GET("/sessions/:id/leaderboard", d.QuizHandler.Leaderboard)
	authed.GET("/ws", d.WSHandler.Handle)

	// Admin + super_admin routes (session management)
	adminGroup := authed.Group("")
	adminGroup.Use(middleware.RequireRole("instructor", "admin", "super_admin"))
	adminGroup.POST("/sessions", d.SessionHandler.Create)
	adminGroup.POST("/sessions/:id/start", d.SessionHandler.Start)
	adminGroup.POST("/sessions/:id/end", d.SessionHandler.End)

	// Super-admin only routes (user management + audit log)
	superAdmin := authed.Group("")
	superAdmin.Use(middleware.RequireRole("super_admin"))
	superAdmin.GET("/users", d.UserHandler.List)
	superAdmin.PATCH("/users/:id/role", d.UserHandler.UpdateRole)
	superAdmin.GET("/audit-logs", d.AuditHandler.List)

	return r
}
