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
		&McpGroupItem{},
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

	// V1.1: uploaded_images 去重改为每用户独立。AutoMigrate 只加不删,这里显式
	// drop 掉旧的 storage_key 全局唯一索引(GORM 默认名 idx_uploaded_images_storage_key),
	// 否则两个用户传同字节仍会触发唯一冲突。新的 (user_id, storage_key) 复合唯一
	// 由 AutoMigrate 按 struct tag 创建。存量数据无需回填(每 key 一行是合法子集)。
	if DB.Migrator().HasIndex(&UploadedImage{}, "idx_uploaded_images_storage_key") {
		if err := DB.Migrator().DropIndex(&UploadedImage{}, "idx_uploaded_images_storage_key"); err != nil {
			return fmt.Errorf("drop legacy storage_key unique index: %w", err)
		}
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

	// V1.4: uploaded_images.short_id 回填 + 唯一索引。short_id 是视觉图片的短 URL
	// 句柄(/u/<sid>)。struct tag 里刻意不写 uniqueIndex: AutoMigrate 会先于回填去
	// 建唯一索引,而存量行此时都还是默认 '',多行 '' 会让 CREATE UNIQUE INDEX 失败、
	// 启动崩溃。正确顺序由下面两个函数保证: 先 AutoMigrate 只加列(无索引) → 回填
	// 空值 → 再建唯一索引。两者各自幂等,可每次启动安全重跑。
	if err := backfillShortIDs(); err != nil {
		return err
	}
	if err := ensureShortIDIndex(); err != nil {
		return err
	}
	return nil
}

// backfillShortIDs populates uploaded_images.short_id for legacy rows (empty from
// the column default) so the unique index can be created afterward. Idempotent:
// it only updates rows whose short_id is still empty, so it is safe to re-run on
// every startup.
func backfillShortIDs() error {
	var rows []UploadedImage
	if err := DB.Select("id").Where("short_id = '' OR short_id IS NULL").Find(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		for i := 0; i < 4; i++ {
			sid, err := NewShortID()
			if err != nil {
				return err
			}
			res := DB.Model(&UploadedImage{}).Where("id = ? AND short_id = ''", r.ID).Update("short_id", sid)
			if err := res.Error; err != nil {
				return err
			}
			if res.RowsAffected > 0 {
				break // wrote this row; move on
			}
			// RowsAffected==0: concurrent backfill or a ~2^-71 collision — regenerate.
		}
	}
	return nil
}

// ensureShortIDIndex creates the unique index on uploaded_images.short_id. It is
// declared in code (not the struct tag) because AutoMigrate would otherwise try
// to build it before backfill populates legacy rows — a unique index on a column
// where every legacy row is empty fails at startup. CREATE UNIQUE INDEX is
// standard SQL (SQLite/MySQL/Postgres); the HasIndex guard makes it idempotent.
func ensureShortIDIndex() error {
	const idx = "idx_uploaded_images_short_id"
	if DB.Migrator().HasIndex(&UploadedImage{}, idx) {
		return nil
	}
	if err := DB.Exec("CREATE UNIQUE INDEX " + idx + " ON uploaded_images (short_id)").Error; err != nil {
		return fmt.Errorf("create short_id unique index: %w", err)
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
