package main

import (
	"log"

	"github.com/di-eqa/backend/internal/router"
	"github.com/gin-gonic/gin"
)

func main() {
	app := Bootstrap()
	defer app.Close()

	gin.SetMode(gin.ReleaseMode)
	r := router.Setup(gin.Default(), app.Deps)

	log.Printf("🚀 DI EQA backend listening on :%s", app.Cfg.Port)
	if err := r.Run(":" + app.Cfg.Port); err != nil {
		log.Fatal(err)
	}
}
