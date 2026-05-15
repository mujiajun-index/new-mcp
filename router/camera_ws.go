package router

import (
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
	cameraID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid camera id"})
		return
	}

	// Verify camera exists
	var cam model.Camera
	if err := model.DB.First(&cam, cameraID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
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

	CameraStream.SetConn(cameraID, conn)
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
