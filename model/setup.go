package model

import (
	"log"
	"time"

	"github.com/mujkjk/newmcp/common"
)

type Setup struct {
	ID            uint   `json:"id" gorm:"primaryKey"`
	Version       string `json:"version" gorm:"type:varchar(50);not null"`
	InitializedAt int64  `json:"initialized_at" gorm:"type:bigint;not null"`
}

func (Setup) TableName() string { return "setup" }

func GetSetup() *Setup {
	var setup Setup
	err := DB.First(&setup).Error
	if err != nil {
		return nil
	}
	return &setup
}

func AdminUserExists() bool {
	var count int64
	DB.Model(&User{}).Where("role = ?", "admin").Count(&count)
	return count > 0
}

func CheckSetup() {
	setup := GetSetup()
	if setup != nil {
		log.Println("[setup] system is already initialized")
		common.SystemInitialized = true
		return
	}

	if AdminUserExists() {
		log.Println("[setup] admin user exists, creating setup record")
		newSetup := Setup{
			Version:       common.Version,
			InitializedAt: time.Now().Unix(),
		}
		DB.Create(&newSetup)
		common.SystemInitialized = true
		return
	}

	log.Println("[setup] system is not initialized, waiting for setup")
	common.SystemInitialized = false
}
