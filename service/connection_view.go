package service

import (
	"errors"
	"net/url"
	"strings"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

// GetForViewer grants administrators read-only, redacted access to other users'
// configurations. All mutation paths continue to use the owner-scoped lookup.
func (s *ConnectionService) GetForViewer(viewerID, connID int64, role string) (*dto.ConnectionDetail, error) {
	if !common.IsAdminRole(role) {
		return s.GetByID(viewerID, connID)
	}
	var conn model.CloudEndpoint
	if err := model.DB.First(&conn, connID).Error; err != nil {
		return nil, err
	}
	detail := s.toDetail(&conn)
	if conn.UserID == viewerID {
		return detail, nil
	}
	detail.ReadOnly = true
	detail.WssURL = maskConnectionURL(conn.WssURL)
	detail.CloudConfig = maskConnectionConfig(detail.CloudConfig)
	// Dial errors can repeat URLs or credentials, so keep only the presence of
	// an error in another user's view rather than returning the raw error text.
	if detail.LastError != "" {
		detail.LastError = secretMask
	}
	var owner model.User
	if err := model.DB.Select("username").First(&owner, conn.UserID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	detail.Username = owner.Username
	if conn.ApiKeyID != nil {
		var key model.ApiKey
		if err := model.DB.Select("name").Where("id = ? AND user_id = ?", *conn.ApiKeyID, conn.UserID).First(&key).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		detail.ApiKeyName = key.Name
	}
	return detail, nil
}

func maskConnectionURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "ws" && u.Scheme != "wss") || u.Opaque != "" {
		return secretMask
	}
	// Custom endpoints can place credentials in paths, query keys/values, user
	// info or fragments. Only the authority is safe to preserve generically.
	masked := u.Scheme + "://"
	if u.User != nil {
		masked += secretMask + "@"
	}
	masked += u.Host
	if u.Path == "/" {
		masked += "/"
	} else if u.Path != "" {
		masked += "/" + secretMask
	}
	if u.RawQuery != "" || u.ForceQuery {
		masked += "?" + secretMask
	}
	if u.Fragment != "" {
		masked += "#" + secretMask
	}
	return masked
}

func maskConnectionConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}
	masked := make(map[string]interface{}, len(config))
	for key, value := range config {
		// SSH's structural fields remain useful for diagnosis; opaque and
		// credential-bearing fields (including nested objects) are fully masked.
		switch strings.ToLower(key) {
		case "host", "port", "username", "auth_type":
			switch value.(type) {
			case string, float64:
				masked[key] = value
				continue
			}
		}
		masked[key] = secretMask
	}
	return masked
}
