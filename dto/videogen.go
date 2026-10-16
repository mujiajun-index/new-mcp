package dto

import "encoding/json"

type VideoGenConfigReq struct {
	Name                       *string          `json:"name"`
	Description                *string          `json:"description"`
	Protocol                   *string          `json:"protocol"`
	ModelName                  *string          `json:"model_name"`
	EndpointURL                *string          `json:"endpoint_url"`
	APIKey                     *string          `json:"api_key"`
	Operation                  *string          `json:"operation"`
	ImageRequestFormat         *string          `json:"image_request_format"`
	DefaultParams              *json.RawMessage `json:"default_params"`
	RequestTimeoutSeconds      *int             `json:"request_timeout_seconds"`
	TaskTimeoutSeconds         *int             `json:"task_timeout_seconds"`
	GenerateVideoDesc          *string          `json:"generate_video_desc"`
	GenerateVideoFromImageDesc *string          `json:"generate_video_from_image_desc"`
	GetVideoStatusDesc         *string          `json:"get_video_status_desc"`
}

type VideoGenProbeReq struct {
	VideoGenConfigReq
	ConfigID  int64           `json:"config_id"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}

type VideoGenRetryReq struct {
	ConfigID  int64           `json:"config_id"`
	RequestID string          `json:"request_id"`
	Arguments json.RawMessage `json:"arguments"`
}
