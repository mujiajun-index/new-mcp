package model

import (
	"time"

	"gorm.io/gorm"
)

// Default tool identity for the vision service's single general-purpose tool.
// The gorm default tags on VisionConfig must stay byte-identical to these.
const (
	DefaultAnalyzeImageName = "analyze_image"
	DefaultAnalyzeImageDesc = "Analyze an image with a vision model. Covers all image understanding: identify objects, people and text, describe the scene and overall content, extract structured info, or answer any custom question. Pass the prompt parameter to steer the analysis, e.g. describe the scene, transcribe all text, or list defects. Returns: the analysis result as text."
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
	AnalyzeTimeoutSeconds int    `json:"analyze_timeout_seconds" gorm:"default:30"`
	AutoRegister          bool   `json:"auto_register" gorm:"default:false"`
	RegisteredServiceID   *int64 `json:"registered_service_id"`
	// Tool identity for the vision service's single general-purpose tool
	// (analyze_image + optional custom prompt). describe_scene was removed —
	// its job is analyze_image with a "describe the scene" prompt — following
	// zai-mcp-server's single-general-tool design. The gorm default tag must
	// stay byte-identical to DefaultAnalyzeImageDesc below.
	AnalyzeImageName string         `json:"analyze_image_name" gorm:"size:128;default:analyze_image"`
	AnalyzeImageDesc string         `json:"analyze_image_desc" gorm:"type:varchar(512);default:Analyze an image with a vision model. Covers all image understanding: identify objects, people and text, describe the scene and overall content, extract structured info, or answer any custom question. Pass the prompt parameter to steer the analysis, e.g. describe the scene, transcribe all text, or list defects. Returns: the analysis result as text."`
	ExtraConfig      string         `json:"extra_config" gorm:"type:varchar(4096);default:'{}'"`
	Status           int            `json:"status" gorm:"default:1"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

func (VisionConfig) TableName() string { return "vision_configs" }

func ListVisionConfigsByUser(userID int64) ([]VisionConfig, error) {
	var configs []VisionConfig
	err := DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&configs).Error
	return configs, err
}

// ListAllVisionConfigs returns every config across users (soft-deleted rows
// excluded by the DeletedAt scope), for the startup tools_cache sync.
func ListAllVisionConfigs() ([]VisionConfig, error) {
	var configs []VisionConfig
	err := DB.Find(&configs).Error
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
