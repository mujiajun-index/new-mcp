package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

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
	defer common.CloseLogFile()

	if err := model.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer model.CloseDB()

	model.CheckSetup()
	model.InitOptionMap()

	gin.SetMode(common.GinMode)

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(middleware.Logger())
	engine.Use(middleware.CORS())

	router.SetRouter(engine)

	// Start cloud connections (XiaoZhi, custom WSS)
	router.StartCloudConnections()
	defer router.StopCloudConnections()

	addr := fmt.Sprintf(":%d", common.Port)
	log.Printf("NewMCP server starting on %s", addr)

	// Graceful shutdown
	srvCtx, srvCancel := context.WithCancel(context.Background())
	defer srvCancel()

	go func() {
		if err := engine.Run(addr); err != nil {
			log.Printf("Server stopped: %v", err)
			srvCancel()
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		log.Println("Shutting down server...")
	case <-srvCtx.Done():
	}

	router.StopCloudConnections()
	model.CloseDB()
	log.Println("Server exited")
}
