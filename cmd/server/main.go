package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/middleware"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/router"
)

func main() {
	_ = godotenv.Load()

	common.InitEnv()

	if err := model.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer model.CloseDB()

	gin.SetMode(common.GinMode)

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(middleware.Logger())
	engine.Use(middleware.CORS())

	router.SetRouter(engine)

	addr := fmt.Sprintf(":%d", common.Port)
	log.Printf("NewMCP server starting on %s", addr)
	if err := engine.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
