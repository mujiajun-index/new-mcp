package model

import "time"

// History survives configuration deletion. It contains no connection snapshot,
// API key, reference bytes or expiring signed links.
type ImageGenTask struct {
	ID             string     `json:"task_id" gorm:"primaryKey;size:36"`
	UserID         int64      `json:"-" gorm:"not null;index:idx_imagegen_task_user_created,priority:1"`
	ConfigID       int64      `json:"config_id" gorm:"index"`
	ConfigName     string     `json:"config_name" gorm:"size:128"`
	Source         string     `json:"source" gorm:"size:16"`
	Operation      string     `json:"operation" gorm:"size:16"`
	Provider       string     `json:"provider" gorm:"size:32"`
	Model          string     `json:"model" gorm:"size:128"`
	Prompt         string     `json:"prompt" gorm:"type:text"`
	ReferenceCount int        `json:"reference_count"`
	Status         string     `json:"status" gorm:"size:16;index"`
	Error          string     `json:"error,omitempty" gorm:"type:text"`
	Warnings       string     `json:"-" gorm:"type:text"`
	Text           string     `json:"text,omitempty" gorm:"type:text"`
	Usage          string     `json:"-" gorm:"type:text"`
	DurationMs     int64      `json:"duration_ms"`
	CreatedAt      time.Time  `json:"created_at" gorm:"index:idx_imagegen_task_user_created,priority:2"`
	UpdatedAt      time.Time  `json:"updated_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

func (ImageGenTask) TableName() string { return "imagegen_tasks" }

type ImageGenTaskImage struct {
	ID            int64  `json:"-" gorm:"primaryKey;autoIncrement"`
	TaskID        string `json:"-" gorm:"size:36;not null;index"`
	UploadID      int64  `json:"-" gorm:"not null;index"`
	MIME          string `json:"mime" gorm:"size:64"`
	Size          int64  `json:"size"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	RevisedPrompt string `json:"revised_prompt,omitempty" gorm:"type:text"`
}

func (ImageGenTaskImage) TableName() string { return "imagegen_task_images" }
