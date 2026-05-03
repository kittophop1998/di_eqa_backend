// Package router wires every HTTP route to its handler.
// main.go is responsible only for bootstrapping dependencies and calling
// router.Setup(); all routing decisions live here.
package router

import (
	"net/http"

	"github.com/di-eqa/backend/internal/config"
	"github.com/di-eqa/backend/internal/handlers"
	"github.com/di-eqa/backend/internal/middleware"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Deps bundles every handler that the router needs.
// Add new handlers here instead of touching Setup's signature.
type Deps struct {
	Cfg            *config.Config
	HospHandler    *handlers.HospitalHandler
	AuthHandler    *handlers.AuthHandler
	QuizHandler    *handlers.QuizHandler
	SessionHandler *handlers.SessionHandler
	WSHandler      *handlers.WSHandler
}

// Setup registers middleware and all route groups onto the provided *gin.Engine
// and returns it so callers can chain further if needed.
func Setup(r *gin.Engine, d Deps) *gin.Engine {
	// ------------------------------------------------------------------ CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
	}))

	// ------------------------------------------------------------------ Health
	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "di-eqa-backend"})
	})

	// ------------------------------------------------------------------ Public
	public := r.Group("/api")
	registerPublicRoutes(public, d)

	// ------------------------------------------------------------------ Authenticated
	authed := r.Group("/api")
	authed.Use(middleware.Auth(d.Cfg.JWTSecret))
	registerAuthedRoutes(authed, d)

	// ------------------------------------------------------------------ Instructor / Admin
	instructor := authed.Group("")
	instructor.Use(middleware.RequireRole("instructor", "admin"))
	registerInstructorRoutes(instructor, d)

	return r
}

// registerPublicRoutes mounts routes that do NOT require authentication.
func registerPublicRoutes(rg *gin.RouterGroup, d Deps) {
	// Hospitals
	rg.GET("/hospitals", d.HospHandler.List)
	rg.GET("/hospitals/:code", d.HospHandler.GetByCode)

	// Auth
	rg.POST("/auth/register", d.AuthHandler.Register)
	rg.POST("/auth/login", d.AuthHandler.Login)
}

// registerAuthedRoutes mounts routes that require a valid JWT.
func registerAuthedRoutes(rg *gin.RouterGroup, d Deps) {
	// Auth
	rg.GET("/auth/me", d.AuthHandler.Me)

	// Quizzes
	rg.GET("/quizzes", d.QuizHandler.List)
	rg.GET("/quizzes/:id", d.QuizHandler.Get)
	rg.POST("/quizzes/:id/submit", d.QuizHandler.Submit)

	// Submissions
	rg.GET("/submissions/me", d.QuizHandler.MyHistory)
	rg.GET("/submissions/:id", d.QuizHandler.GetSubmission)

	// Sessions (read)
	rg.GET("/sessions/active", d.SessionHandler.ListActive)
	rg.GET("/sessions/:id", d.SessionHandler.Get)
	rg.GET("/sessions/code/:code", d.SessionHandler.GetByCode)
	rg.GET("/sessions/:id/leaderboard", d.QuizHandler.Leaderboard)

	// WebSocket
	rg.GET("/ws", d.WSHandler.Handle)
}

// registerInstructorRoutes mounts routes restricted to instructor / admin roles.
func registerInstructorRoutes(rg *gin.RouterGroup, d Deps) {
	rg.POST("/sessions", d.SessionHandler.Create)
	rg.POST("/sessions/:id/start", d.SessionHandler.Start)
	rg.POST("/sessions/:id/end", d.SessionHandler.End)
}
