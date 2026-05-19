package main

import (
	"log"

	adaptercache "github.com/di-eqa/backend/internal/adapter/cache"
	adapthttp "github.com/di-eqa/backend/internal/adapter/http/handler"
	"github.com/di-eqa/backend/internal/adapter/http/router"
	mongorepo "github.com/di-eqa/backend/internal/adapter/repository/mongo"
	adaptws "github.com/di-eqa/backend/internal/adapter/ws"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/di-eqa/backend/internal/infrastructure/cache"
	"github.com/di-eqa/backend/internal/infrastructure/config"
	"github.com/di-eqa/backend/internal/infrastructure/db"
	"github.com/di-eqa/backend/internal/infrastructure/seed"
	"github.com/redis/go-redis/v9"
)

// App holds all initialised dependencies for the server lifetime.
type App struct {
	Cfg         *config.Config
	MongoConn   *db.Mongo
	RedisClient *redis.Client
	Hub         *adaptws.Hub
	Deps        router.Deps
}

func Bootstrap() *App {
	// 1. Configuration
	cfg := config.Load()
	log.Println("✅ config loaded")

	// 2. MongoDB
	mongoConn, err := db.Connect(cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		log.Fatalf("❌ mongodb connect error: %v", err)
	}
	log.Println("✅ mongodb connected")

	// 3. Redis
	redisClient, err := cache.Connect(cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		log.Fatalf("❌ redis connect error: %v", err)
	}
	log.Println("✅ redis connected")

	// 4. Seed initial data (non-fatal)
	if err := seed.Run(mongoConn.DB, cfg.SupabaseURL, cfg.SupabaseBucket); err != nil {
		log.Printf("⚠️  seed warning: %v", err)
	} else {
		log.Println("✅ seed completed")
	}

	// 5. WebSocket hub (implements port.EventPort)
	hub := adaptws.NewHub()
	log.Println("✅ ws hub initialised")

	// 6. Wire repository adapters (driven)
	database := mongoConn.DB
	userRepo := mongorepo.NewUserRepo(database.Collection("users"))
	hospitalRepo := mongorepo.NewHospitalRepo(database.Collection("hospitals"))
	quizRepo := mongorepo.NewQuizRepo(database.Collection("quizzes"))
	sessionRepo := mongorepo.NewSessionRepo(database.Collection("sessions"))
	submissionRepo := mongorepo.NewSubmissionRepo(database.Collection("submissions"))
	auditRepo := mongorepo.NewAuditLogRepo(database.Collection("audit_logs"))

	// 7. Wire cache adapter (driven)
	cacheAdapter := adaptercache.NewRedisCache(redisClient)

	// 8. Wire application services (use cases)
	auditSvc := service.NewAuditService(auditRepo)
	authSvc := service.NewAuthService(userRepo, hospitalRepo, auditSvc, cfg)
	hospitalSvc := service.NewHospitalService(hospitalRepo)
	userMgmtSvc := service.NewUserService(userRepo, hospitalRepo, auditSvc)
	quizSvc := service.NewQuizService(
		quizRepo, sessionRepo, submissionRepo, userRepo, hospitalRepo,
		cacheAdapter, hub, cfg.SupabaseURL, cfg.SupabaseBucket,
	)
	sessionSvc := service.NewSessionService(sessionRepo, quizRepo, hospitalRepo, hub)

	// 9. Wire HTTP handler adapters (driving)
	wsLookup := adapthttp.NewWSUserLookup(userRepo, hospitalRepo)
	deps := router.Deps{
		Cfg:            cfg,
		Redis:          redisClient,
		HospHandler:    adapthttp.NewHospitalHandler(hospitalSvc),
		AuthHandler:    adapthttp.NewAuthHandler(authSvc),
		QuizHandler:    adapthttp.NewQuizHandler(quizSvc),
		SessionHandler: adapthttp.NewSessionHandler(sessionSvc),
		WSHandler:      adapthttp.NewWSHandler(hub, wsLookup),
		UserHandler:    adapthttp.NewUserHandler(userMgmtSvc),
		AuditHandler:   adapthttp.NewAuditHandler(auditSvc),
	}

	return &App{
		Cfg:         cfg,
		MongoConn:   mongoConn,
		RedisClient: redisClient,
		Hub:         hub,
		Deps:        deps,
	}
}

// Close releases all resources held by App.
func (a *App) Close() {
	a.MongoConn.Close()
}
