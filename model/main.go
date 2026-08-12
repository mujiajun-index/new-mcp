package model

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/common"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB() error {
	var dialector gorm.Dialector

	switch common.DbType {
	case "mysql":
		dialector = mysql.Open(common.SqlDSN)
	case "postgres":
		dialector = postgres.Open(common.SqlDSN)
	default:
		if err := os.MkdirAll(filepath.Dir(common.DbPath), 0755); err != nil {
			return fmt.Errorf("create data directory: %w", err)
		}
		dialector = sqlite.Open(common.DbPath)
	}

	var logLevel logger.LogLevel
	switch common.LogLevel {
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	case "info":
		logLevel = logger.Info
	default:
		logLevel = logger.Info
	}

	var err error
	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(10)

	if err := migrateDB(); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	return nil
}

func migrateDB() error {
	if err := DB.AutoMigrate(
		&Setup{},
		&User{},
		&ApiKey{},
		&McpService{},
		&McpGroup{},
		&McpGroupService{},
		&McpGroupTool{},
		&VisionConfig{},
		&Camera{},
		&CloudEndpoint{},
		&McpCallLog{},
		&MarketplaceItem{},
		&MarketplaceGroup{},
		&MarketplaceTag{},
		&MarketplaceReview{},
		&McpToolPrice{},
		&Redemption{},
		&Option{},
		&UploadedImage{},
	); err != nil {
		return err
	}
	// 统一日志回填:历史 mcp_call_logs 行均为 MCP 调用,
	// type 列新增后(default:2)兜底把任何 0/NULL 行置为 Consume。
	if err := DB.Model(&McpCallLog{}).Where("type = 0 OR type IS NULL").Update("type", LogTypeConsume).Error; err != nil {
		return err
	}

	// 邀请码回填:为历史存量用户(aff_code 列新增后为 NULL/空)补发邀请码,
	// 使其立即可分享,无需等首次打开钱包页才惰性生成。仅补空值,不覆盖已有码。
	var usersNeedingCode []User
	if err := DB.Select("id").Where("aff_code = '' OR aff_code IS NULL").Find(&usersNeedingCode).Error; err != nil {
		return err
	}
	for _, u := range usersNeedingCode {
		code, err := GenerateAffCode()
		if err != nil {
			return err
		}
		if err := DB.Model(&User{}).Where("id = ?", u.ID).Update("aff_code", code).Error; err != nil {
			return err
		}
	}
	return nil
}

func CloseDB() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
