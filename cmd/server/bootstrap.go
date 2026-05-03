package main

import (
	"log"

	"github.com/di-eqa/backend/internal/cache"
	"github.com/di-eqa/backend/internal/config"
	"github.com/di-eqa/backend/internal/db"
	"github.com/di-eqa/backend/internal/handlers"
	"github.com/di-eqa/backend/internal/router"
	"github.com/di-eqa/backend/internal/seed"
	"github.com/di-eqa/backend/internal/ws"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
)

// App holds all initialised dependencies for the server lifetime.
type App struct {
	Cfg         *config.Config
	MongoConn   *db.Mongo
	RedisClient *redis.Client
	Hub         *ws.Hub
	Deps        router.Deps
}

// Bootstrap initialises every external dependency in order and returns a
// fully-wired *App ready to serve requests.
// Call app.Close() in a defer to release resources on shutdown.
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
	if err := seed.Run(mongoConn.DB); err != nil {
		log.Printf("⚠️  seed warning: %v", err)
	} else {
		log.Println("✅ seed completed")
	}

	// 5. WebSocket hub
	hub := ws.NewHub()
	log.Println("✅ ws hub initialised")

	// 6. Wire collections → handlers → router deps
	colls := initCollections(mongoConn.DB)
	deps := buildDeps(cfg, colls, redisClient, hub)

	return &App{
		Cfg:         cfg,
		MongoConn:   mongoConn,
		RedisClient: redisClient,
		Hub:         hub,
		Deps:        deps,
	}
}

// Close releases all resources held by App.
// Intended to be called with defer in main().
func (a *App) Close() {
	a.MongoConn.Close()
}

// buildDeps wires handlers into the router.Deps struct.
func buildDeps(
	cfg *config.Config,
	colls *collections,
	redisClient *redis.Client,
	hub *ws.Hub,
) router.Deps {
	return router.Deps{
		Cfg: cfg,
		HospHandler: &handlers.HospitalHandler{
			Coll: colls.hospitals,
		},
		AuthHandler: &handlers.AuthHandler{
			Cfg:          cfg,
			UsersColl:    colls.users,
			HospitalColl: colls.hospitals,
		},
		QuizHandler: &handlers.QuizHandler{
			QuizColl:       colls.quizzes,
			SubmissionColl: colls.submissions,
			HospitalColl:   colls.hospitals,
			UsersColl:      colls.users,
			SessionColl:    colls.sessions,
			Redis:          redisClient,
			Hub:            hub,
		},
		SessionHandler: &handlers.SessionHandler{
			SessionColl: colls.sessions,
			QuizColl:    colls.quizzes,
			Hub:         hub,
		},
		WSHandler: &handlers.WSHandler{
			Hub:          hub,
			UsersColl:    colls.users,
			HospitalColl: colls.hospitals,
		},
	}
}

// collections is a small private bundle that keeps collection references together.
type collections struct {
	hospitals   *mongo.Collection
	users       *mongo.Collection
	quizzes     *mongo.Collection
	submissions *mongo.Collection
	sessions    *mongo.Collection
}
