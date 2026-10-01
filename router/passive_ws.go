package router

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/service"
)

var passiveUpgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
}

func HandlePassiveWebSocket(c *gin.Context) {
	if SessionPool == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "接入服务尚未初始化"})
		return
	}
	token := c.Query("token")
	svc, err := service.ValidatePassiveToken(token)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, service.ErrPassiveDisabled) {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	conn, err := passiveUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	adapter := transport.NewPassiveWSAdapter(svc.ID, conn)
	defer adapter.Close()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	err = adapter.Connect(ctx)
	cancel()
	if err != nil {
		log.Printf("[passive-ws] service %d handshake failed", svc.ID)
		return
	}
	err = SessionPool.WithServiceLock(svc.ID, func() error {
		current, err := service.ValidatePassiveToken(token)
		if err != nil {
			return err
		}
		_, err = SessionPool.AttachPassiveLocked(current, adapter)
		return err
	})
	if err != nil {
		log.Printf("[passive-ws] service %d connection rejected after handshake", svc.ID)
		return
	}
	// Keep the upgraded handler alive independently of individual tool requests.
	<-adapter.Done()
}
