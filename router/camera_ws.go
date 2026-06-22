package router

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/middleware"
	"github.com/mujkjk/newmcp/model"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func HandleCameraStream(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证信息"})
		return
	}

	claims, err := middleware.ParseToken(tokenStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的认证令牌"})
		return
	}

	user, err := model.GetUserByID(claims.UserID)
	if err != nil || user.Status != common.StatusEnabled {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在或已禁用"})
		return
	}
	_ = user

	cameraID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid camera id"})
		return
	}

	// Verify camera exists AND belongs to the authenticated user (prevent IDOR:
	// without the user_id filter any valid token could stream any camera by id).
	cam, err := model.GetCameraByID(claims.UserID, cameraID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	// 拒绝已禁用的摄像头推流
	if !cam.AutoRegister {
		c.JSON(http.StatusForbidden, gin.H{"error": "摄像头已禁用，请先启用后再推流"})
		return
	}

	// 同一摄像头同一时刻只允许一个推流连接，避免重复推流
	if CameraStream != nil && CameraStream.IsStreaming(cameraID) {
		c.JSON(http.StatusConflict, gin.H{"error": "该摄像头正在推流中，请先停止现有推流"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[camera-ws] upgrade failed: %v", err)
		return
	}

	if CameraStream == nil {
		conn.Close()
		return
	}

	// 原子占用推流通道；若在 HTTP 检查与升级之间有其他连接抢先占用（竞态），拒绝本连接
	if !CameraStream.TryAcquire(cameraID, conn) {
		conn.Close()
		return
	}
	defer func() {
		CameraStream.Cleanup(cameraID)
	}()

	log.Printf("[camera-ws] camera %d stream connected", cameraID)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[camera-ws] camera %d read error: %v", cameraID, err)
			}
			break
		}
		CameraStream.HandleFrame(cameraID, message)
	}

	log.Printf("[camera-ws] camera %d stream disconnected", cameraID)
}
