package model

import (
	"time"

	"gorm.io/gorm"
)

type Camera struct {
	ID                 int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID             int64          `json:"user_id" gorm:"not null;index"`
	Name               string         `json:"name" gorm:"size:128;not null"`
	Description        string         `json:"description" gorm:"type:text"`
	SourceType         string         `json:"source_type" gorm:"size:32;not null"`
	SourceURL          string         `json:"source_url" gorm:"size:512;not null"`
	FPS                float64        `json:"fps" gorm:"type:decimal(4,1);default:1.0"`
	ResolutionW        int            `json:"resolution_w" gorm:"default:640"`
	ResolutionH        int            `json:"resolution_h" gorm:"default:480"`
	VisionConfigID     *int64         `json:"vision_config_id" gorm:"index"`
	AutoRegister       bool           `json:"auto_register" gorm:"default:false"`
	RegisteredServiceID *int64        `json:"registered_service_id"`
	CaptureName        string         `json:"capture_name" gorm:"size:128;default:camera.capture"`
	CaptureDesc        string         `json:"capture_desc" gorm:"type:varchar(512);default:Capture a single still frame from the live camera feed and return it as an image. Best for: taking snapshots or capturing the current view. Returns: the captured frame as an image."`
	AnalyzeName        string         `json:"analyze_name" gorm:"size:128;default:camera.analyze"`
	AnalyzeDesc        string         `json:"analyze_desc" gorm:"type:varchar(512);default:Capture the current camera frame and run visual analysis on it. Best for: detecting objects, people, or events in the live feed. Returns: the analysis result for the current frame."`
	ExtraConfig        string         `json:"extra_config" gorm:"type:varchar(4096);default:'{}'"`
	Status             int            `json:"status" gorm:"default:1"`
	LastCaptureAt      *time.Time     `json:"last_capture_at"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Camera) TableName() string { return "cameras" }

func ListCamerasByUser(userID int64) ([]Camera, error) {
	var cameras []Camera
	err := DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&cameras).Error
	return cameras, err
}

func GetCameraByID(userID, id int64) (*Camera, error) {
	var camera Camera
	err := DB.Where("id = ? AND user_id = ?", id, userID).First(&camera).Error
	return &camera, err
}

func GetCameraByServiceID(serviceID int64) (*Camera, error) {
	var camera Camera
	err := DB.Where("registered_service_id = ?", serviceID).First(&camera).Error
	return &camera, err
}

func (c *Camera) Insert() error {
	return DB.Create(c).Error
}

func (c *Camera) Update() error {
	return DB.Save(c).Error
}

func (c *Camera) Delete() error {
	return DB.Delete(c).Error
}
