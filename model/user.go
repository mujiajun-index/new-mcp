package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID           int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	Username     string         `json:"username" gorm:"uniqueIndex;size:64;not null"`
	Password     string         `json:"-" gorm:"not null"`
	DisplayName  string         `json:"display_name" gorm:"size:128"`
	Email        string         `json:"email" gorm:"size:255"`
	Role         string         `json:"role" gorm:"size:32;default:user"`
	Status       int            `json:"status" gorm:"default:1"`
	AvatarURL    string         `json:"avatar_url" gorm:"column:avatar_url;size:512"`
	Quota        int64          `json:"quota" gorm:"default:0"`
	UsedQuota    int64          `json:"used_quota" gorm:"default:0"`
	RequestCount int64          `json:"request_count" gorm:"default:0"`
	Group        string         `json:"group" gorm:"size:64;default:default"`
	Remark       string         `json:"remark" gorm:"size:255"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

func (User) TableName() string { return "users" }

func GetUserByUsername(username string) (*User, error) {
	var user User
	err := DB.Where("username = ?", username).First(&user).Error
	return &user, err
}

// IsEmailAlreadyTaken reports whether an account with the given email already exists.
func IsEmailAlreadyTaken(email string) bool {
	var count int64
	DB.Model(&User{}).Where("email = ?", email).Count(&count)
	return count > 0
}

// GetUserByUsernameOrEmail looks up a user by username OR email, so users can
// sign in with either identifier.
func GetUserByUsernameOrEmail(identifier string) (*User, error) {
	var user User
	err := DB.Where("username = ? OR email = ?", identifier, identifier).First(&user).Error
	return &user, err
}

func GetUserByID(id int64) (*User, error) {
	var user User
	err := DB.First(&user, id).Error
	return &user, err
}

func ListUsersWithPaged(offset, limit int, keyword string, excludeID int64) ([]User, int64, error) {
	var users []User
	var total int64
	query := DB.Model(&User{})
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	if keyword != "" {
		query = query.Where("username LIKE ? OR email LIKE ? OR display_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Offset(offset).Limit(limit).Order("id ASC").Find(&users).Error
	return users, total, err
}

func IncreaseUserQuota(id int64, quota int64) error {
	return DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota)).Error
}

func IncreaseUserRequestCount(id int64) error {
	return DB.Model(&User{}).Where("id = ?", id).Update("request_count", gorm.Expr("request_count + ?", 1)).Error
}

func DecreaseUserQuota(id int64, quota int64) error {
	return DB.Model(&User{}).Where("id = ? AND quota >= ?", id, quota).Update("quota", gorm.Expr("quota - ?", quota)).Error
}

func (u *User) Insert() error {
	return DB.Create(u).Error
}

func (u *User) Update() error {
	return DB.Save(u).Error
}
