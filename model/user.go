package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	Username  string         `json:"username" gorm:"uniqueIndex;size:64;not null"`
	Password  string         `json:"-" gorm:"not null"`
	Email     string         `json:"email" gorm:"size:255"`
	Role      string         `json:"role" gorm:"size:32;default:user"`
	Status    int            `json:"status" gorm:"default:1"`
	AvatarURL string         `json:"avatar_url" gorm:"column:avatar_url;size:512"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (User) TableName() string { return "users" }

func GetUserByUsername(username string) (*User, error) {
	var user User
	err := DB.Where("username = ?", username).First(&user).Error
	return &user, err
}

func GetUserByID(id int64) (*User, error) {
	var user User
	err := DB.First(&user, id).Error
	return &user, err
}

func (u *User) Insert() error {
	return DB.Create(u).Error
}

func (u *User) Update() error {
	return DB.Save(u).Error
}
