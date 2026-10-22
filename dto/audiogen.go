package dto

import (
	"encoding/json"
	"github.com/mujkjk/newmcp/model"
)

type AudioGenConnectionReq struct {
	Key      string  `json:"key"`
	Name     string  `json:"name"`
	Protocol string  `json:"protocol"`
	BaseURL  string  `json:"base_url"`
	APIKey   *string `json:"api_key"`
}
type AudioGenModelReq struct {
	Kind           string                      `json:"kind"`
	Enabled        bool                        `json:"enabled"`
	ConnectionKey  string                      `json:"connection_key"`
	Model          string                      `json:"model"`
	DefaultParams  json.RawMessage             `json:"default_params"`
	TimeoutSeconds int                         `json:"timeout_seconds"`
	Description    string                      `json:"description"`
	Capabilities   *model.AudioGenCapabilities `json:"capabilities"`
}
type AudioGenConfigReq struct {
	Name        *string                  `json:"name"`
	Description *string                  `json:"description"`
	Connections *[]AudioGenConnectionReq `json:"connections"`
	Models      *[]AudioGenModelReq      `json:"models"`
}
type AudioGenProbeReq struct {
	AudioGenConfigReq
	ConfigID      int64           `json:"config_id"`
	Kind          string          `json:"kind"`
	ConnectionKey string          `json:"connection_key"`
	Arguments     json.RawMessage `json:"arguments"`
}
type AudioGenRetryReq struct {
	ConfigID  int64           `json:"config_id"`
	Arguments json.RawMessage `json:"arguments"`
}
