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
	return DB.AutoMigrate(
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
		&MarketplaceReview{},
		&Option{},
	)
}

func CloseDB() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
