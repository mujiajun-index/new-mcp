package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	GenerateVideoToolName                       = "generate_video"
	GenerateVideoFromImageToolName              = "generate_video_from_image"
	GetVideoStatusToolName                      = "get_video_status"
	DefaultGenerateVideoDesc                    = "Create a new video from a text prompt using the configured video model. Describe the scene, motion, camera and style in prompt. Optional seconds, size and metadata depend on the model. Returns a task_id and progress; generation continues after this call. Call get_video_status with that task_id until the video is saved and a downloadable resource_link is returned. wait_seconds is a short wait, not a generation timeout. Reuse request_id when retrying the same submission to avoid creating another paid upstream job."
	DefaultGenerateVideoFromImageDesc           = "Animate a single reference image into a video. Pass prompt describing motion and image as an HTTP(S) URL, data URL or small base64 image. For a LOCAL file, call upload_image, run its returned upload_command, then pass image_url as image. The server cannot read your local disk. Returns a task_id; call get_video_status until the video is ready. Model-specific parameters belong in metadata or extra_body. Reuse request_id when retrying the same submission."
	DefaultGetVideoStatusDesc                   = "Check a video task created by this service using its local task_id. Returns generation status, progress, artifact_status and saved video links. Running tasks and short-wait expiry are successful responses. If saving failed after generation completed, set retry_download=true to retry saving without generating or paying for a new video. Querying a paused task resumes tracking. Never resubmit generate_video just to check progress."
	DefaultVideoUploadDesc                      = "Stage a LOCAL reference image for video generation. Call with local_path, run the returned upload_command using your shell, then call generate_video_from_image with the returned image_url as image. The server cannot read your local files directly."
	DefaultOpenRouterGenerateVideoDesc          = "Create a new video from a text prompt using the configured OpenRouter video model. Describe the scene, motion, camera and style in prompt. Optional seconds must be an integer and is sent as duration. Put resolution, aspect_ratio, generate_audio, seed and provider options in extra_body; metadata is not supported. Returns a task_id; call get_video_status until the video is saved. wait_seconds is only a short wait. Reuse request_id when retrying the same submission to avoid another paid upstream job."
	DefaultOpenRouterGenerateVideoFromImageDesc = "Animate a single first-frame reference image using the configured OpenRouter video model. Pass prompt and image as an HTTP(S) URL, data URL or small base64 image. For a LOCAL file, call upload_image, run its returned upload_command, then use image_url as image. The image is sent in JSON frame_images as first_frame. Optional integer seconds is sent as duration; model-specific options belong in extra_body, not metadata. Returns a task_id; call get_video_status until ready. Reuse request_id when retrying the same submission."
	DefaultGeminiGenerateVideoDesc              = "Create a new video from a text prompt using the configured Gemini native Veo model. Optional integer seconds maps to durationSeconds; size maps to resolution (for example 720p). Put native camelCase generation parameters such as aspectRatio, negativePrompt, personGeneration and seed directly in extra_body, which becomes parameters; metadata is not supported. One video is saved per task. Returns a task_id; call get_video_status until the video is saved. wait_seconds is only a short wait. Reuse request_id when retrying the same submission to avoid another paid upstream job."
	DefaultGeminiGenerateVideoFromImageDesc     = "Animate a single first-frame image using the configured Gemini native Veo model. Pass prompt and image as an HTTP(S) URL, data URL or small base64 image. For a LOCAL file, call upload_image, run the returned upload_command, then pass image_url. The server validates and embeds the image bytes as bytesBase64Encoded with mimeType in JSON. Optional integer seconds maps to durationSeconds; size maps to resolution. Put native camelCase generation parameters directly in extra_body, not metadata. Video editing, extension and additional reference frames are not supported. Returns a task_id; query get_video_status until ready. Reuse request_id for the same submission."
	DefaultMiniMaxGenerateVideoDesc             = "Create a video using the configured MiniMax native v2 model or compatible self-hosted deployment. prompt becomes a text item in content. Optional integer seconds maps to duration (default 5); size maps to resolution (default 768P). Put ratio (default 16:9), extra.prompt_expansion_mode and other native generation options in extra_body; metadata is not supported. Model names are sent unchanged. Returns a local task_id; call get_video_status until saved. wait_seconds is only a short wait. Reuse request_id for the same submission to avoid another paid generation."
	DefaultMiniMaxGenerateVideoFromImageDesc    = "Animate a single first-frame image using the configured MiniMax native v2 video model. Pass prompt and image as an HTTP(S) URL, data URL or small base64 image; upload local files using upload_image first. The server validates the image and embeds its bytes as a data URL in content with role first_frame. Optional integer seconds maps to duration (default 5), size maps to resolution (default 768P), and ratio defaults to adaptive. Put native generation options in extra_body, not metadata. Additional frames, video editing and extension are not supported. Returns a local task_id; query get_video_status until saved. Reuse request_id for the same submission."
)

type VideoGenConfig struct {
	ID                         int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID                     int64          `json:"-" gorm:"not null;index"`
	Name                       string         `json:"name" gorm:"size:128;not null"`
	Description                string         `json:"description" gorm:"type:text"`
	Protocol                   string         `json:"protocol" gorm:"size:32;not null"`
	ModelName                  string         `json:"model_name" gorm:"size:128;not null"`
	EndpointURL                string         `json:"endpoint_url" gorm:"size:512;not null"`
	APIKey                     string         `json:"-" gorm:"type:text"`
	Operation                  string         `json:"operation" gorm:"size:16;not null"`
	ImageRequestFormat         string         `json:"image_request_format" gorm:"size:16;not null"`
	DefaultParams              string         `json:"default_params" gorm:"type:text"`
	RequestTimeoutSeconds      int            `json:"request_timeout_seconds"`
	TaskTimeoutSeconds         int            `json:"task_timeout_seconds"`
	GenerateVideoDesc          string         `json:"generate_video_desc" gorm:"type:text"`
	GenerateVideoFromImageDesc string         `json:"generate_video_from_image_desc" gorm:"type:text"`
	GetVideoStatusDesc         string         `json:"get_video_status_desc" gorm:"type:text"`
	AutoRegister               bool           `json:"auto_register"`
	RegisteredServiceID        *int64         `json:"registered_service_id" gorm:"uniqueIndex"`
	Status                     int            `json:"status"`
	CreatedAt                  time.Time      `json:"created_at"`
	UpdatedAt                  time.Time      `json:"updated_at"`
	DeletedAt                  gorm.DeletedAt `json:"-" gorm:"index"`
}

func (VideoGenConfig) TableName() string { return "videogen_configs" }

func GetVideoGenConfig(uid, id int64) (*VideoGenConfig, error) {
	var c VideoGenConfig
	err := DB.Where("user_id = ? AND id = ?", uid, id).First(&c).Error
	return &c, err
}

func GetVideoGenConfigByServiceID(id int64) (*VideoGenConfig, error) {
	var c VideoGenConfig
	err := DB.Where("registered_service_id = ?", id).First(&c).Error
	return &c, err
}

// Task rows outlive configurations. Connection is a private snapshot using
// common.Encrypt's existing CRYPTO_SECRET semantics, never API JSON.
type VideoGenTask struct {
	ServiceID             int64      `json:"-" gorm:"index"`
	MarketplaceItemID     *int64     `json:"-" gorm:"index"`
	ID                    string     `json:"task_id" gorm:"primaryKey;size:36"`
	UserID                int64      `json:"-" gorm:"not null;index"`
	ConfigID              int64      `json:"config_id" gorm:"index"`
	ConfigName            string     `json:"config_name" gorm:"size:128"`
	Source                string     `json:"source" gorm:"size:16"`
	Operation             string     `json:"operation" gorm:"size:16"`
	Protocol              string     `json:"protocol" gorm:"size:32;not null;default:openai_video"`
	Model                 string     `json:"model" gorm:"size:128"`
	Prompt                string     `json:"prompt" gorm:"type:text"`
	ReferenceImages       string     `json:"-" gorm:"type:text"`
	IdempotencyKey        string     `json:"-" gorm:"size:64;not null;uniqueIndex"`
	RequestHash           string     `json:"-" gorm:"size:64"`
	RequestParams         string     `json:"-"`
	RetryRequests         string     `json:"-"`
	RetryCount            int        `json:"retry_count,omitempty" gorm:"not null;default:0"`
	RetryOf               string     `json:"retry_of,omitempty" gorm:"size:36;index"`
	Connection            string     `json:"-" gorm:"type:text"`
	UpstreamID            string     `json:"-" gorm:"size:512"`
	Status                string     `json:"status" gorm:"size:32;index"`
	Progress              int        `json:"progress"`
	ArtifactStatus        string     `json:"artifact_status" gorm:"size:16;index"`
	TrackingPaused        bool       `json:"tracking_paused"`
	Error                 string     `json:"error,omitempty" gorm:"type:text"`
	LastError             string     `json:"last_error,omitempty" gorm:"type:text"`
	RequestTimeoutSeconds int        `json:"-"`
	TaskTimeoutSeconds    int        `json:"-"`
	TrackingDeadline      time.Time  `json:"-"`
	NextPollAt            time.Time  `json:"-" gorm:"index"`
	LeaseOwner            string     `json:"-" gorm:"size:64"`
	Revision              int64      `json:"-" gorm:"not null;default:0"`
	LeaseUntil            time.Time  `json:"-" gorm:"index"`
	Failures              int        `json:"-"`
	StorageKey            string     `json:"-" gorm:"size:128"`
	Backend               string     `json:"-" gorm:"size:16"`
	ShortID               *string    `json:"-" gorm:"size:16;uniqueIndex"`
	MIME                  string     `json:"mime,omitempty" gorm:"size:64"`
	Size                  int64      `json:"size,omitempty"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty" gorm:"index"`
	StoppedAt             *time.Time `json:"-" gorm:"index"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (VideoGenTask) TableName() string { return "videogen_tasks" }

func GetVideoGenTask(uid int64, id string) (*VideoGenTask, error) {
	var t VideoGenTask
	err := DB.Where("id = ? AND user_id = ?", id, uid).First(&t).Error
	return &t, err
}
