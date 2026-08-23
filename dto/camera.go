package dto

type CreateCameraReq struct {
	Name          string `json:"name" binding:"required,min=1,max=128"`
	Description   string `json:"description"`
	VisionConfigID int64 `json:"vision_config_id" binding:"required"`
}

type UpdateCameraReq struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	VisionConfigID *int64 `json:"vision_config_id"`
	CaptureDesc  *string `json:"capture_desc"`
	AnalyzeDesc  *string `json:"analyze_desc"`
	Status       *int    `json:"status"`
}

type CameraListItem struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	VisionConfigID     *int64 `json:"vision_config_id"`
	VisionConfigName   string `json:"vision_config_name"`
	AutoRegister       bool   `json:"auto_register"`
	RegisteredServiceID *int64 `json:"registered_service_id"`
	Streaming          bool   `json:"streaming"`
	HasStreamKey       bool   `json:"has_stream_key"`
	StreamKeyExpiresAt string `json:"stream_key_expires_at"` // 空串=永久或未生成;不含密钥本身
	Status             int    `json:"status"`
	CreatedAt          string `json:"created_at"`
}

type CameraDetail struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	VisionConfigID      *int64 `json:"vision_config_id"`
	VisionConfigName    string `json:"vision_config_name"`
	AutoRegister        bool   `json:"auto_register"`
	RegisteredServiceID *int64  `json:"registered_service_id"`
	CaptureDesc         string `json:"capture_desc"`
	AnalyzeDesc         string `json:"analyze_desc"`
	ExtraConfig         string `json:"extra_config"`
	Streaming           bool   `json:"streaming"`
	HasStreamKey        bool   `json:"has_stream_key"`
	StreamKeyExpiresAt  string `json:"stream_key_expires_at"` // 空串=永久或未生成;不含密钥本身
	Status              int    `json:"status"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

// StreamKeyReq 生成/重生成推流密钥的请求体
type StreamKeyReq struct {
	ExpiresIn int64 `json:"expires_in"` // 有效期(秒),0=永久
}

// CameraStreamKey 推流密钥信息(属主鉴权后返回,含完整推流页链接)
type CameraStreamKey struct {
	StreamKey string `json:"stream_key"`
	StreamURL string `json:"stream_url"`
	ExpiresAt string `json:"expires_at"` // 空串=永久
	HasKey    bool   `json:"has_key"`
}
