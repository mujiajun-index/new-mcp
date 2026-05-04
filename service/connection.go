package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/cloud"
	"github.com/mujkjk/newmcp/model"
)

// CloudManager is set during application startup
var CloudManager *cloud.Manager

type ConnectionService struct{}

func (s *ConnectionService) List(userID int64) ([]dto.ConnectionListItem, error) {
	conns, err := model.ListConnectionsByUser(userID)
	if err != nil {
		return nil, err
	}

	items := make([]dto.ConnectionListItem, len(conns))
	for i, c := range conns {
		items[i] = dto.ConnectionListItem{
			ID:               c.ID,
			Name:             c.Name,
			CloudType:        c.CloudType,
			RemoteID:         c.RemoteID,
			ConnectionStatus: c.ConnectionStatus,
			AutoConnect:      c.AutoConnect,
			CreatedAt:        c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, nil
}

func (s *ConnectionService) Create(userID int64, req *dto.CreateConnectionReq) (*dto.ConnectionDetail, error) {
	cloudConfigJSON := "{}"
	if req.CloudConfig != nil {
		b, _ := json.Marshal(req.CloudConfig)
		cloudConfigJSON = string(b)
	}

	autoConnect := true
	if req.AutoConnect != nil {
		autoConnect = *req.AutoConnect
	}

	conn := &model.CloudEndpoint{
		UserID:           userID,
		Name:             req.Name,
		CloudType:        req.CloudType,
		WssURL:           req.WssURL,
		CloudConfig:      cloudConfigJSON,
		ApiKeyID:         req.ApiKeyID,
		AutoConnect:      autoConnect,
		ConnectionStatus: common.ConnDisconnected,
		Status:           common.StatusEnabled,
	}

	if req.CloudType == "xiaozhi" && req.WssURL != "" {
		s.parseXiaoZhiToken(conn, req.WssURL)
	}

	if err := conn.Insert(); err != nil {
		return nil, err
	}

	// Auto-connect if requested
	if autoConnect && CloudManager != nil {
		go CloudManager.StartEndpoint(conn)
	}

	return s.toDetail(conn), nil
}

func (s *ConnectionService) GetByID(userID, connID int64) (*dto.ConnectionDetail, error) {
	conn, err := model.GetConnectionByID(userID, connID)
	if err != nil {
		return nil, err
	}
	return s.toDetail(conn), nil
}

func (s *ConnectionService) Update(userID, connID int64, req *dto.UpdateConnectionReq) error {
	conn, err := model.GetConnectionByID(userID, connID)
	if err != nil {
		return err
	}
	if req.Name != nil {
		conn.Name = *req.Name
	}
	if req.WssURL != nil {
		conn.WssURL = *req.WssURL
	}
	if req.ApiKeyID != nil {
		conn.ApiKeyID = req.ApiKeyID
	}
	if req.Status != nil {
		conn.Status = *req.Status
	}
	return conn.Update()
}

func (s *ConnectionService) Delete(userID, connID int64) error {
	conn, err := model.GetConnectionByID(userID, connID)
	if err != nil {
		return err
	}
	if CloudManager != nil {
		CloudManager.StopEndpoint(connID)
	}
	return conn.Delete()
}

func (s *ConnectionService) Connect(userID, connID int64) error {
	conn, err := model.GetConnectionByID(userID, connID)
	if err != nil {
		return err
	}
	if CloudManager == nil {
		conn.ConnectionStatus = common.ConnConnected
		now := time.Now()
		conn.LastConnectedAt = &now
		return conn.Update()
	}
	return CloudManager.RestartEndpoint(conn)
}

func (s *ConnectionService) Disconnect(userID, connID int64) error {
	conn, err := model.GetConnectionByID(userID, connID)
	if err != nil {
		return err
	}
	if CloudManager != nil {
		CloudManager.StopEndpoint(connID)
	}
	conn.ConnectionStatus = common.ConnDisconnected
	return conn.Update()
}

func (s *ConnectionService) BindApiKey(userID, connID int64, apiKeyID int64) error {
	conn, err := model.GetConnectionByID(userID, connID)
	if err != nil {
		return err
	}
	conn.ApiKeyID = &apiKeyID
	if err := conn.Update(); err != nil {
		return err
	}
	// Re-register tools with new API key permissions
	if CloudManager != nil {
		go CloudManager.RestartEndpoint(conn)
	}
	return nil
}

// parseXiaoZhiToken extracts agentId and exp from the JWT in the WSS URL query
func (s *ConnectionService) parseXiaoZhiToken(conn *model.CloudEndpoint, wssURL string) {
	u, err := url.Parse(wssURL)
	if err != nil {
		return
	}
	tokenStr := u.Query().Get("token")
	if tokenStr == "" {
		return
	}

	claims, err := decodeJWTPayload(tokenStr)
	if err != nil {
		return
	}

	if agentID, ok := claims["agentId"]; ok {
		switch v := agentID.(type) {
		case float64:
			conn.RemoteID = strconv.FormatInt(int64(v), 10)
		case string:
			conn.RemoteID = v
		}
	}

	if exp, ok := claims["exp"]; ok {
		switch v := exp.(type) {
		case float64:
			t := time.Unix(int64(v), 0)
			conn.TokenExpiresAt = &t
		}
	}

	if endpointID, ok := claims["endpointId"]; ok {
		if conn.RemoteID == "" {
			switch v := endpointID.(type) {
			case string:
				conn.RemoteID = v
			}
		}
	}
}

// decodeJWTPayload decodes JWT payload without signature verification
func decodeJWTPayload(tokenStr string) (map[string]interface{}, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	payload := parts[1]
	// Add padding if needed
	if m := len(payload) % 4; m != 0 {
		payload += strings.Repeat("=", 4-m)
	}

	data, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// Try standard encoding
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("decode payload: %w", err)
		}
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	return claims, nil
}

func (s *ConnectionService) toDetail(conn *model.CloudEndpoint) *dto.ConnectionDetail {
	var cloudConfig map[string]interface{}
	_ = json.Unmarshal([]byte(conn.CloudConfig), &cloudConfig)

	var tokenExpiresAt, lastConnectedAt string
	if conn.TokenExpiresAt != nil {
		tokenExpiresAt = conn.TokenExpiresAt.Format("2006-01-02T15:04:05Z")
	}
	if conn.LastConnectedAt != nil {
		lastConnectedAt = conn.LastConnectedAt.Format("2006-01-02T15:04:05Z")
	}

	return &dto.ConnectionDetail{
		ID:               conn.ID,
		Name:             conn.Name,
		CloudType:        conn.CloudType,
		WssURL:           conn.WssURL,
		CloudConfig:      cloudConfig,
		RemoteID:         conn.RemoteID,
		TokenExpiresAt:   tokenExpiresAt,
		ApiKeyID:         conn.ApiKeyID,
		AutoConnect:      conn.AutoConnect,
		ConnectionStatus: conn.ConnectionStatus,
		LastConnectedAt:  lastConnectedAt,
		LastError:        conn.LastError,
	}
}
