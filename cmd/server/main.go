package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/di-eqa/backend/internal/adapter/http/router"
	"github.com/gin-gonic/gin"
)

func main() {
	app := Bootstrap()
	defer app.Close()

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	_ = engine.SetTrustedProxies(nil) // ClientIP() = socket peer; never trust client-supplied forwarding headers
	router.Setup(engine, app.Deps)

	srv := &http.Server{
		Addr:              ":" + app.Cfg.Port,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("DI EQA backend listening on :%s", app.Cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
