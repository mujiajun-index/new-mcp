package camera

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type CameraStream struct {
	LatestFrame []byte
	CapturedAt  time.Time
	Conn        *websocket.Conn
}

type CameraStreamManager struct {
	mu      sync.RWMutex
	streams map[int64]*CameraStream
}

func NewCameraStreamManager() *CameraStreamManager {
	return &CameraStreamManager{
		streams: make(map[int64]*CameraStream),
	}
}

func (m *CameraStreamManager) HandleFrame(cameraID int64, frame []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.streams[cameraID]
	if !ok {
		s = &CameraStream{}
		m.streams[cameraID] = s
	}
	s.LatestFrame = frame
	s.CapturedAt = time.Now()
}

func (m *CameraStreamManager) SetConn(cameraID int64, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.streams[cameraID]
	if !ok {
		s = &CameraStream{}
		m.streams[cameraID] = s
	}
	s.Conn = conn
}

func (m *CameraStreamManager) GetLatestFrame(cameraID int64) ([]byte, time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.streams[cameraID]
	if !ok || len(s.LatestFrame) == 0 {
		return nil, time.Time{}, false
	}
	return s.LatestFrame, s.CapturedAt, true
}

func (m *CameraStreamManager) IsStreaming(cameraID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.streams[cameraID]
	return ok && s.Conn != nil
}

func (m *CameraStreamManager) Cleanup(cameraID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.streams[cameraID]; ok {
		if s.Conn != nil {
			s.Conn.Close()
		}
		delete(m.streams, cameraID)
	}
}
