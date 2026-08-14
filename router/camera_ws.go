package router

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mujkjk/newmcp/model"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func HandleCameraStream(c *gin.Context) {
	// auth via query stream key (?k=) — WebSocket can't send custom headers.
	// 密钥即凭证:持有正确密钥 = 授权推流该摄像头,归属校验由密钥本身承担(IDOR 防御)。
	key := c.Query("k")
	if key == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供推流密钥"})
		return
	}

	cameraID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid camera id"})
		return
	}

	cam, err := model.GetCameraByIDAny(cameraID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}

	// 常数时间比对,避免时序侧信道(密钥定长 22,与 VerifyShort 同款写法)
	if subtle.ConstantTimeCompare([]byte(cam.StreamKey), []byte(key)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的推流密钥"})
		return
	}
	if !cam.StreamKeyValid() {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "推流密钥已过期或已撤销"})
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
