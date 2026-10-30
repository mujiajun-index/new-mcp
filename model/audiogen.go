package model

import (
	"gorm.io/gorm"
	"time"
)

type AudioGenCapabilities struct {
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	MaxSpeakers      int      `json:"max_speakers"`
	Lyrics           bool     `json:"lyrics"`
	Instrumental     bool     `json:"instrumental"`
	LyricsOptimizer  bool     `json:"lyrics_optimizer"`
}

type AudioGenConfig struct {
	ID                  int64                `json:"id" gorm:"primaryKey"`
	UserID              int64                `json:"-" gorm:"index;not null"`
	Name                string               `json:"name" gorm:"size:128"`
	Description         string               `json:"description" gorm:"type:text"`
	AutoRegister        bool                 `json:"auto_register"`
	RegisteredServiceID *int64               `json:"registered_service_id" gorm:"uniqueIndex"`
	Status              int                  `json:"status"`
	Connections         []AudioGenConnection `json:"connections" gorm:"foreignKey:ConfigID"`
	Models              []AudioGenModel      `json:"models" gorm:"foreignKey:ConfigID"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
	DeletedAt           gorm.DeletedAt       `json:"-" gorm:"index"`
}

func (AudioGenConfig) TableName() string { return "audiogen_configs" }

type AudioGenConnection struct {
	ID        int64  `json:"id" gorm:"primaryKey"`
	ConfigID  int64  `json:"-" gorm:"uniqueIndex:idx_audio_connection,priority:1"`
	Key       string `json:"key" gorm:"size:64;uniqueIndex:idx_audio_connection,priority:2"`
	Name      string `json:"name" gorm:"size:128"`
	Protocol  string `json:"protocol" gorm:"size:40;not null"`
	BaseURL   string `json:"base_url" gorm:"size:512"`
	APIKey    string `json:"-" gorm:"type:text"`
	HasAPIKey bool   `json:"has_api_key" gorm:"-"`
}

func (AudioGenConnection) TableName() string { return "audiogen_connections" }

type AudioGenModel struct {
	ID             int64                `json:"id" gorm:"primaryKey"`
	ConfigID       int64                `json:"-" gorm:"uniqueIndex:idx_audio_slot,priority:1"`
	Kind           string               `json:"kind" gorm:"size:16;uniqueIndex:idx_audio_slot,priority:2"`
	Enabled        bool                 `json:"enabled"`
	ConnectionKey  string               `json:"connection_key" gorm:"size:64"`
	Protocol       string               `json:"protocol" gorm:"size:40"`
	Model          string               `json:"model" gorm:"size:128"`
	DefaultParams  string               `json:"default_params" gorm:"type:text"`
	TimeoutSeconds int                  `json:"timeout_seconds"`
	Description    string               `json:"description" gorm:"type:text"`
	Capabilities   AudioGenCapabilities `json:"capabilities" gorm:"serializer:json;type:text"`
}

func (AudioGenModel) TableName() string { return "audiogen_models" }

type AudioGenTask struct {
	ServiceID         int64     `json:"-" gorm:"index"`
	MarketplaceItemID *int64    `json:"-" gorm:"index"`
	ID                string    `json:"task_id" gorm:"primaryKey;size:36"`
	UserID            int64     `json:"-" gorm:"index"`
	ConfigID          int64     `json:"config_id" gorm:"index"`
	ExecutionConfigID int64     `json:"execution_config_id"`
	ConfigName        string    `json:"config_name"`
	Kind              string    `json:"kind" gorm:"size:16;index"`
	Source            string    `json:"source" gorm:"size:16"`
	Protocol          string    `json:"protocol"`
	Model             string    `json:"model"`
	Prompt            string    `json:"prompt" gorm:"type:text"`
	Status            string    `json:"status" gorm:"size:32;index"`
	ArtifactStatus    string    `json:"artifact_status" gorm:"size:16;index"`
	Error             string    `json:"error,omitempty" gorm:"type:text"`
	LastError         string    `json:"last_error,omitempty" gorm:"type:text"`
	IdempotencyKey    string    `json:"-" gorm:"size:64;uniqueIndex"`
	RequestHash       string    `json:"-" gorm:"size:64"`
	Connection        string    `json:"-" gorm:"type:text"`
	RequestParams     string    `json:"-" gorm:"type:text"`
	InputKey          string    `json:"-"`
	OutputKey         string    `json:"-"`
	ReferenceImages   string    `json:"-" gorm:"type:text"`
	ReferenceAudios   string    `json:"-" gorm:"type:text"`
	ResultText        string    `json:"text,omitempty" gorm:"type:text"`
	Transcript        string    `json:"transcript,omitempty" gorm:"type:text"`
	InputTranscript   string    `json:"input_transcript,omitempty" gorm:"type:text"`
	Lyrics            string    `json:"lyrics,omitempty" gorm:"type:text"`
	Usage             string    `json:"-" gorm:"type:text"`
	RetryCount        int       `json:"retry_count"`
	LeaseOwner        string    `json:"-" gorm:"size:36"`
	LeaseUntil        time.Time `json:"-" gorm:"index"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (AudioGenTask) TableName() string { return "audiogen_tasks" }

type AudioAsset struct {
	ID         int64      `json:"id" gorm:"primaryKey"`
	UserID     int64      `json:"-" gorm:"index"`
	TaskID     string     `json:"task_id,omitempty" gorm:"size:36;index"`
	ShortID    string     `json:"-" gorm:"size:16;uniqueIndex"`
	StorageKey string     `json:"-"`
	Source     string     `json:"source" gorm:"size:16;index"`
	Status     string     `json:"status" gorm:"size:16;index"`
	MIME       string     `json:"mime"`
	Size       int64      `json:"size"`
	ExpiresAt  *time.Time `json:"expires_at" gorm:"index"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (AudioAsset) TableName() string { return "audio_assets" }

func GetAudioGenConfig(uid, id int64) (*AudioGenConfig, error) {
	var c AudioGenConfig
	err := DB.Preload("Connections").Preload("Models").Where("user_id = ? AND id = ?", uid, id).First(&c).Error
	return &c, err
}
