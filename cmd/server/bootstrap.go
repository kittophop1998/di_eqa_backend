package main

import (
	"context"
	"log"
	"time"

	"github.com/di-eqa/backend/internal/adapter/http/handler"
	"github.com/di-eqa/backend/internal/adapter/http/router"
	mongorepo "github.com/di-eqa/backend/internal/adapter/repository/mongo"
	"github.com/di-eqa/backend/internal/adapter/security"
	"github.com/di-eqa/backend/internal/adapter/storage"
	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/application/service"
	"github.com/di-eqa/backend/internal/infrastructure/cache"
	"github.com/di-eqa/backend/internal/infrastructure/config"
	"github.com/di-eqa/backend/internal/infrastructure/db"
	"github.com/di-eqa/backend/internal/infrastructure/migrate"
	"github.com/redis/go-redis/v9"
)

// App holds all initialised dependencies for the server lifetime.
type App struct {
	Cfg         *config.Config
	MongoConn   *db.Mongo
	RedisClient *redis.Client
	Deps        router.Deps
}

// Bootstrap is the composition root: it loads configuration, connects
// datastores, checks migrations and wires adapters into services and handlers.
// It contains wiring only, no business rules.
func Bootstrap() *App {
	// 1. Configuration (fails fast on a missing/weak secret in production)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}
	for _, w := range cfg.Warnings {
		log.Printf("WARN: %s", w)
	}
	log.Printf("config loaded (env=%s)", cfg.Env)

	// 2. Datastores
	mongoConn, err := db.Connect(cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		log.Fatalf("FATAL: mongodb connect: %v", err)
	}
	redisClient, err := cache.Connect(cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		log.Fatalf("FATAL: redis connect: %v", err)
	}
	database := mongoConn.DB

	// 3. Migrations: apply when RUN_MIGRATIONS=true, otherwise refuse to serve
	// on an unmigrated database (run cmd/migrate first).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	store := migrate.NewMongoStore(database)
	steps := migrate.Steps(database)
	if cfg.RunMigrations {
		if _, err := migrate.Run(ctx, store, steps, log.Printf); err != nil {
			log.Fatalf("FATAL: %v", err)
		}
	} else if pending, err := migrate.Pending(ctx, store, steps); err != nil {
		log.Fatalf("FATAL: %v", err)
	} else if len(pending) > 0 {
		log.Fatalf("FATAL: %d database migration(s) pending (first: %d %s). Run `go run ./cmd/migrate` or start with RUN_MIGRATIONS=true",
			len(pending), pending[0].Version, pending[0].Name)
	}

	// 4. Driven adapters
	userRepo := mongorepo.NewUserRepo(database.Collection("users"))
	hospitalRepo := mongorepo.NewHospitalRepo(database.Collection("hospitals"))
	cellTypeRepo := mongorepo.NewCellTypeRepo(database.Collection("cell_types"))
	cellImageRepo := mongorepo.NewCellImageRepo(database.Collection("cell_images"))
	quizRepo := mongorepo.NewQuizRepo(database.Collection("quizzes"))
	assignmentRepo := mongorepo.NewAssignmentRepo(database.Collection("quiz_assignments"))
	attemptRepo := mongorepo.NewAttemptRepo(database.Collection("attempts"))
	certRepo := mongorepo.NewCertificateRepo(database.Collection("certificates"))
	auditRepo := mongorepo.NewAuditLogRepo(database.Collection("audit_logs"))

	jwtService := security.NewJWTService(cfg.JWTSecret, cfg.JWTExpiry)
	hasher := security.NewBcryptHasher(0)
	rnd := security.NewCryptoRandomness()
	clock := security.SystemClock{}
	images := buildImageStore(cfg)

	// 5. Application services
	auditSvc := service.NewAuditService(auditRepo)
	authSvc := service.NewAuthService(userRepo, hospitalRepo, auditSvc, jwtService, hasher, clock)
	hospitalSvc := service.NewHospitalService(hospitalRepo, userRepo, assignmentRepo, auditSvc, clock)
	adminUserSvc := service.NewAdminUserService(userRepo, hospitalRepo, auditSvc, clock)
	cellSvc := service.NewCellLibraryService(cellTypeRepo, cellImageRepo, images)
	quizAdminSvc := service.NewQuizAdminService(quizRepo, assignmentRepo, hospitalRepo, userRepo, attemptRepo,
		cellTypeRepo, cellImageRepo, auditSvc, clock)
	myQuizSvc := service.NewMyQuizService(quizRepo, assignmentRepo, attemptRepo, certRepo, clock)
	attemptSvc := service.NewAttemptService(quizRepo, assignmentRepo, attemptRepo, certRepo, hospitalRepo,
		cellTypeRepo, cellImageRepo, images, rnd, clock)
	certSvc := service.NewCertificateService(certRepo)

	// 6. Initial super_admin from env (only when none exists)
	created, err := service.NewAdminBootstrap(userRepo, hasher, clock).
		EnsureSuperAdmin(ctx, cfg.AdminInitialUsername, cfg.AdminInitialPassword)
	if err != nil {
		log.Fatalf("FATAL: admin bootstrap: %v", err)
	}
	if created {
		log.Printf("created initial super_admin %q; remove ADMIN_INITIAL_PASSWORD from the environment", cfg.AdminInitialUsername)
	}

	// 7. Driving adapters
	deps := router.Deps{
		AllowedOrigins:  cfg.AllowedOrigins,
		Redis:           redisClient,
		TokenVerifier:   jwtService,
		Authenticator:   authSvc,
		AuthHandler:     handler.NewAuthHandler(authSvc),
		HospitalHandler: handler.NewHospitalHandler(hospitalSvc),
		MeHandler:       handler.NewMeHandler(myQuizSvc, attemptSvc, certSvc),
		AdminHandler:    handler.NewAdminHandler(adminUserSvc, cellSvc, quizAdminSvc),
		AuditHandler:    handler.NewAuditHandler(auditSvc),
	}
	return &App{Cfg: cfg, MongoConn: mongoConn, RedisClient: redisClient, Deps: deps}
}

// buildImageStore prefers a local directory (dev) and falls back to Supabase.
func buildImageStore(cfg *config.Config) applicationport.ImageStore {
	var stores []applicationport.ImageStore
	if cfg.ImageLocalDir != "" {
		stores = append(stores, storage.NewLocalStore(cfg.ImageLocalDir))
	}
	if cfg.SupabaseURL != "" {
		stores = append(stores, storage.NewSupabaseStore(cfg.SupabaseURL, cfg.SupabaseBucket))
	}
	return storage.NewChain(stores...)
}

// Close releases all resources held by App.
func (a *App) Close() {
	a.MongoConn.Close()
	_ = a.RedisClient.Close()
}
