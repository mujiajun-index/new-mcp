package service

import (
	"encoding/json"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

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

	// Parse XiaoZhi JWT to extract remote_id and expires
	if req.CloudType == "xiaozhi" && req.WssURL != "" {
		s.parseXiaoZhiToken(conn, req.WssURL)
	}

	if err := conn.Insert(); err != nil {
		return nil, err
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
	return conn.Delete()
}

func (s *ConnectionService) Connect(userID, connID int64) error {
	// Actual WSS connection will be implemented in Phase 4
	conn, err := model.GetConnectionByID(userID, connID)
	if err != nil {
		return err
	}
	conn.ConnectionStatus = common.ConnConnected
	now := time.Now()
	conn.LastConnectedAt = &now
	return conn.Update()
}

func (s *ConnectionService) Disconnect(userID, connID int64) error {
	conn, err := model.GetConnectionByID(userID, connID)
	if err != nil {
		return err
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
	return conn.Update()
}

func (s *ConnectionService) parseXiaoZhiToken(conn *model.CloudEndpoint, wssURL string) {
	// Extract JWT token from WSS URL and parse claims
	// to get remote_id (Agent ID) and token expiration
	// Actual JWT parsing will be implemented in Phase 4
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
