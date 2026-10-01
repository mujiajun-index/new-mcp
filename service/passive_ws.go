package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

var ErrPassiveCredentials = errors.New("无效的接入凭证")
var ErrPassiveDisabled = errors.New("服务或所属用户已禁用")

// The service ID locates the record; only the independent random secret grants
// access. This credential is unrelated to login tokens and gateway API keys.
func newPassiveToken() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return common.Encrypt(base64.RawURLEncoding.EncodeToString(secret))
}

func preparePassiveService(svc *model.McpService, req *dto.CreateServiceReq) error {
	if svc.TransportType != common.TransportPassiveWS {
		return nil
	}
	if req.KeyMode != "" || len(req.AuthKeys) != 0 {
		return fmt.Errorf("被动 WebSocket 接入使用独立接入凭证")
	}
	if _, err := passiveBaseURL(); err != nil {
		return err
	}
	token, err := newPassiveToken()
	if err != nil {
		return err
	}
	svc.PassiveToken = token
	svc.Config, svc.AuthType, svc.AuthConfig = "{}", "none", "{}"
	return nil
}

func passiveBaseURL() (*url.URL, error) {
	u, err := url.Parse(model.GetOptionString("ServerAddress"))
	if err != nil || u == nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("请在系统设置中配置有效的服务器地址")
	}
	switch u.Scheme {
	case "https", "wss":
		u.Scheme = "wss"
	case "http", "ws":
		u.Scheme = "ws"
	default:
		return nil, fmt.Errorf("服务器地址须使用 HTTP 或 HTTPS")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/mcp/passive/"
	u.RawPath = ""
	return u, nil
}

func passiveURL(svc *model.McpService) (string, error) {
	u, err := passiveBaseURL()
	if err != nil {
		return "", err
	}
	secret, err := common.Decrypt(svc.PassiveToken)
	if err != nil || secret == "" {
		return "", fmt.Errorf("无法读取接入凭证，请重置接入地址")
	}
	query := u.Query()
	query.Set("token", strconv.FormatInt(svc.ID, 10)+"."+secret)
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// InitializePassiveServices upgrades placeholder records and discards online
// flags from the previous process. Existing credentials and catalogs survive.
func InitializePassiveServices() error {
	if err := model.DB.Model(&model.McpService{}).
		Where("transport_type = ?", common.TransportPassiveWS).
		Updates(map[string]interface{}{"passive_connected": false, "health_status": common.HealthUnknown,
			"config": "{}", "auth_type": "none", "auth_config": "{}"}).Error; err != nil {
		return err
	}
	var services []model.McpService
	if err := model.DB.Select("id").Where("transport_type = ? AND (passive_token = '' OR passive_token IS NULL)", common.TransportPassiveWS).
		Find(&services).Error; err != nil {
		return err
	}
	for _, svc := range services {
		token, err := newPassiveToken()
		if err != nil {
			return err
		}
		if err := model.DB.Model(&model.McpService{}).Where("id = ?", svc.ID).Update("passive_token", token).Error; err != nil {
			return err
		}
	}
	return nil
}

// ValidatePassiveToken is used both before upgrade and, under the service lock,
// after the MCP handshake so a concurrent reset/disable cannot publish a session.
func ValidatePassiveToken(token string) (*model.McpService, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, ErrPassiveCredentials
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return nil, ErrPassiveCredentials
	}
	var svc model.McpService
	if err := model.DB.First(&svc, id).Error; err != nil || svc.TransportType != common.TransportPassiveWS {
		return nil, ErrPassiveCredentials
	}
	secret, err := common.Decrypt(svc.PassiveToken)
	if err != nil || len(secret) != 43 || subtle.ConstantTimeCompare([]byte(secret), []byte(parts[1])) != 1 {
		return nil, ErrPassiveCredentials
	}
	user, err := model.GetUserByID(svc.UserID)
	if err != nil || user.Status != common.StatusEnabled || svc.Status != common.StatusEnabled {
		return nil, ErrPassiveDisabled
	}
	if svc.Source == "user" && !model.GetOptionBool("UserOwnedServicesEnabled") {
		return nil, ErrPassiveDisabled
	}
	return &svc, nil
}

func withServiceLock(serviceID int64, fn func() error) error {
	if SessionPool != nil {
		return SessionPool.WithServiceLock(serviceID, fn)
	}
	return fn()
}

func passiveConnected(svc *model.McpService) bool {
	if svc.Status != common.StatusEnabled || SessionPool == nil {
		return false
	}
	session := SessionPool.Get(svc.ID)
	return session != nil && session.Adapter.IsConnected()
}

func passiveHealth(svc *model.McpService, connected bool) string {
	if connected {
		return common.HealthHealthy
	}
	if svc.Status != common.StatusEnabled || svc.ProtocolVersion == "" {
		return common.HealthUnknown
	}
	return common.HealthUnhealthy
}

func (s *McpServiceService) testPassive(svc *model.McpService) *dto.TestResult {
	result := &dto.TestResult{Connected: false, Error: "等待服务接入"}
	if SessionPool == nil || svc.Status != common.StatusEnabled {
		return result
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	session, release, err := SessionPool.Acquire(ctx, svc)
	if err != nil {
		return result
	}
	defer release()
	start := time.Now()
	if pinger, ok := session.Adapter.(interface{ Ping(context.Context) error }); ok {
		if err := pinger.Ping(ctx); err != nil {
			result.Error = err.Error()
			return result
		}
	}
	return &dto.TestResult{Connected: session.Adapter.IsConnected(), ToolsCount: len(session.Adapter.GetTools()),
		LatencyMs: time.Since(start).Milliseconds(), ProtocolVersion: session.Adapter.GetProtocolVersion(), ServerInfo: handshakeInfoMap(session.Adapter)}
}

func (s *McpServiceService) ResetPassiveToken(userID, serviceID int64) (*dto.ServiceDetail, error) {
	var result *dto.ServiceDetail
	err := withServiceLock(serviceID, func() error {
		svc, err := model.GetServiceByID(userID, serviceID)
		if err != nil {
			return err
		}
		if svc.TransportType != common.TransportPassiveWS {
			return fmt.Errorf("仅被动 WebSocket 服务支持重置接入地址")
		}
		if _, err := passiveBaseURL(); err != nil {
			return err
		}
		token, err := newPassiveToken()
		if err != nil {
			return err
		}
		if err := model.DB.Model(&model.McpService{}).Where("id = ?", serviceID).
			Updates(map[string]interface{}{"passive_token": token, "passive_connected": false, "health_status": common.HealthUnknown}).Error; err != nil {
			return err
		}
		if SessionPool != nil {
			SessionPool.Remove(serviceID)
		}
		svc.PassiveToken = token
		svc.PassiveConnected = false
		svc.HealthStatus = common.HealthUnknown
		result = s.toDetail(svc)
		return nil
	})
	return result, err
}
