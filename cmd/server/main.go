package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	defer router.StopGateway()

	// Start cloud connections (XiaoZhi, custom WSS)
	router.StartCloudConnections()
	defer router.StopCloudConnections()

	// Graceful shutdown
	srvCtx, srvCancel := context.WithCancel(context.Background())
	defer srvCancel()

	// Background workers (expired-upload cleanup, …) — bound to srvCtx so they
	// exit on shutdown. Started after srvCtx exists for the same reason.
	router.StartBackgroundJobs(srvCtx)
	defer router.StopBackgroundJobs()

	addr := fmt.Sprintf(":%d", common.Port)
	log.Printf("NewMCP server starting on %s", addr)

	srv := &http.Server{
		Addr: addr, Handler: engine,
		BaseContext: func(net.Listener) context.Context { return srvCtx },
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

	// Cancel HTTP/SSE and hijacked WebSocket lifetimes before draining requests.
	srvCancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown: %v", err)
	}
	shutdownCancel()
	router.StopCloudConnections()
	router.StopBackgroundJobs()
	router.StopGateway()
	log.Println("Server exited")
}
