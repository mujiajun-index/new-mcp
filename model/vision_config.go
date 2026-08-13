package model

import (
	"time"

	"gorm.io/gorm"
)

type VisionConfig struct {
	ID           int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID       int64  `json:"user_id" gorm:"not null;index"`
	Name         string `json:"name" gorm:"size:128;not null"`
	Description  string `json:"description" gorm:"type:text"`
	Provider     string `json:"provider" gorm:"size:32;not null;index"`
	ModelName    string `json:"model_name" gorm:"size:128"`
	EndpointURL  string `json:"endpoint_url" gorm:"size:512"`
	ApiKey       string `json:"-" gorm:"column:api_key;size:512"`
	SystemPrompt string `json:"system_prompt" gorm:"type:text"`
	// MaxTokens == 0 means "unlimited"; omitting the GORM default lets 0 persist
	// (GORM would otherwise rewrite the zero value to its column default).
	MaxTokens int `json:"max_tokens"`
	// AnalyzeTimeoutSeconds bounds each image-analysis call to the upstream
	// provider, in seconds. The column defaults to 30 (AutoMigrate also
	// backfills pre-existing rows with 30 via ADD COLUMN ... DEFAULT 30), so a
	// newly-created config — which leaves this at 0 — gets 30s. 0 means "no
	// timeout": doPost then skips the context deadline and the call runs under the
	// caller's existing ctx (or none for MCP tool calls). The create flow does not
	// expose this field; users set it (incl. 0) on the detail page, whose DB.Save
	// writes zero values as-is.
	AnalyzeTimeoutSeconds int            `json:"analyze_timeout_seconds" gorm:"default:30"`
	AutoRegister          bool           `json:"auto_register" gorm:"default:false"`
	RegisteredServiceID   *int64         `json:"registered_service_id"`
	AnalyzeImageName      string         `json:"analyze_image_name" gorm:"size:128;default:vision.analyze_image"`
	AnalyzeImageDesc      string         `json:"analyze_image_desc" gorm:"type:varchar(512);default:Analyze image content and identify the objects, text, and scenes it contains. Best for: extracting structured info, detecting items, or reading text. Returns: a detailed breakdown of recognized elements."`
	DescribeSceneName     string         `json:"describe_scene_name" gorm:"size:128;default:vision.describe_scene"`
	DescribeSceneDesc     string         `json:"describe_scene_desc" gorm:"type:varchar(512);default:Describe the scene and overall content of an image in natural language. Best for: getting a high-level summary of what is happening. Returns: a natural-language description of the scene."`
	ExtraConfig           string         `json:"extra_config" gorm:"type:varchar(4096);default:'{}'"`
	Status                int            `json:"status" gorm:"default:1"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `json:"-" gorm:"index"`
}

func (VisionConfig) TableName() string { return "vision_configs" }

func ListVisionConfigsByUser(userID int64) ([]VisionConfig, error) {
	var configs []VisionConfig
	err := DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&configs).Error
	return configs, err
}

func GetVisionConfigByID(userID, id int64) (*VisionConfig, error) {
	var config VisionConfig
	err := DB.Where("id = ? AND user_id = ?", id, userID).First(&config).Error
	return &config, err
}

func GetVisionConfigByServiceID(serviceID int64) (*VisionConfig, error) {
	var config VisionConfig
	err := DB.Where("registered_service_id = ?", serviceID).First(&config).Error
	return &config, err
}

func (v *VisionConfig) Insert() error {
	return DB.Create(v).Error
}

func (v *VisionConfig) Update() error {
	return DB.Save(v).Error
}

func (v *VisionConfig) Delete() error {
	return DB.Delete(v).Error
}
