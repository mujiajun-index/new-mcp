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
	CaptureName  *string `json:"capture_name"`
	CaptureDesc  *string `json:"capture_desc"`
	AnalyzeName  *string `json:"analyze_name"`
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
	RegisteredServiceID *int64 `json:"registered_service_id"`
	CaptureName         string `json:"capture_name"`
	CaptureDesc         string `json:"capture_desc"`
	AnalyzeName         string `json:"analyze_name"`
	AnalyzeDesc         string `json:"analyze_desc"`
	ExtraConfig         string `json:"extra_config"`
	Streaming           bool   `json:"streaming"`
	Status              int    `json:"status"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}
