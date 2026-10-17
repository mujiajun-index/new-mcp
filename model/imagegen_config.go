package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	GenerateImageToolName     = "generate_image"
	EditImageToolName         = "edit_image"
	GetImageStatusToolName    = "get_image_status"
	ImageGenAsyncDescSuffix   = " Generation runs as a persistent background task. By default this call waits up to 10 seconds; set wait_seconds from 0 to 30. If still processing, returns task_id normally; call get_image_status with that ID until images are ready. Client disconnect does not cancel accepted work. Reuse request_id for the same submission to avoid duplicate paid generation; do not call generation again just to query progress."
	DefaultGetImageStatusDesc = "Query an image task by its local task_id. Returns queued/running/saving or the saved image links. wait_seconds optionally waits 0-30 seconds. Use retry_download=true only when can_retry_save=true to retry storing already-generated images without generating or paying again. Querying never resubmits generation. Only tasks owned by this user and this configuration are accessible. Optional return_mode=inline includes small saved MCP images."
	DefaultGenerateImageDesc  = "Generate new images from a text prompt with an image generation model. Covers image creation: illustrations, photorealistic scenes, product images, posters, icons, and other visual assets supported by the configured model. Pass prompt to describe the subject, composition, style, colors, and any text to include. For changes based on existing images, use edit_image when available. Returns: downloadable image URLs and metadata. Set return_mode=inline to also include native MCP image content when size limits allow." + ImageGenAsyncDescSuffix
	DefaultEditImageDesc      = "Edit images or create variations from reference images with an image generation model. Covers image transformations: add or remove objects, replace backgrounds, change styles or colors, and combine references, as supported by the configured model. Pass prompt to describe the requested changes and images with one or more HTTP(S) URLs, data URLs, or small base64 images. For LOCAL files, call upload_image and run its returned upload_command first. If this tool exposes mask, provide an optional PNG matching the first image's dimensions; transparent areas are edited on supported OpenAI-compatible models. Masks require the standard multipart edits interface; Gemini and JSON generations editing do not support masks. Returns: downloadable image URLs and metadata. Set return_mode=inline to also include native MCP image content when size limits allow." + ImageGenAsyncDescSuffix
	DefaultImageGenUploadDesc = "Stage a LOCAL reference image or PNG mask for image editing. Workflow: 1) call this with local_path; 2) run the returned upload_command via your Bash/shell tool; 3) call edit_image with the returned image_url in images or mask. The server cannot read your local files directly. For SMALL images you may instead pass base64 directly to edit_image; use this tool for larger images."
)

type ImageGenConfig struct {
	ID                  int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID              int64          `json:"-" gorm:"not null;index"`
	Name                string         `json:"name" gorm:"size:128;not null"`
	Description         string         `json:"description" gorm:"type:text"`
	Provider            string         `json:"provider" gorm:"size:32;not null"`
	ModelName           string         `json:"model_name" gorm:"size:128;not null"`
	EndpointURL         string         `json:"endpoint_url" gorm:"size:512;not null"`
	ApiKey              string         `json:"-" gorm:"size:512"`
	Operation           string         `json:"operation" gorm:"size:16;not null"`
	EditRequestFormat   string         `json:"edit_request_format" gorm:"size:32"`
	DefaultParams       string         `json:"default_params" gorm:"type:text"`
	TimeoutSeconds      int            `json:"timeout_seconds"`
	GenerateImageDesc   string         `json:"generate_image_desc" gorm:"type:text"`
	EditImageDesc       string         `json:"edit_image_desc" gorm:"type:text"`
	GetImageStatusDesc  string         `json:"get_image_status_desc" gorm:"type:text"`
	AutoRegister        bool           `json:"auto_register"`
	RegisteredServiceID *int64         `json:"registered_service_id" gorm:"uniqueIndex"`
	Status              int            `json:"status"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `json:"-" gorm:"index"`
}

func (ImageGenConfig) TableName() string { return "imagegen_configs" }

func GetImageGenConfig(userID, id int64) (*ImageGenConfig, error) {
	var cfg ImageGenConfig
	err := DB.Where("user_id = ? AND id = ?", userID, id).First(&cfg).Error
	return &cfg, err
}

func GetImageGenConfigByServiceID(id int64) (*ImageGenConfig, error) {
	var cfg ImageGenConfig
	err := DB.Where("registered_service_id = ?", id).First(&cfg).Error
	return &cfg, err
}
