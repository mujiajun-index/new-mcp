package service

import (
	"context"
	"log"
	"time"

	"github.com/mujkjk/newmcp/model"
)

type MaintenanceService struct{}

// CleanupExpiredLogs 按日志 TTL(LogRetentionDays)清理过期调用日志(§4.5)。
// LogRetentionDays<=0 表示永久保留,直接返回。返回删除行数。
func CleanupExpiredLogs() (int64, error) {
	days := model.GetOptionInt("LogRetentionDays")
	if days <= 0 {
		return 0, nil // 永久保留
	}
	cutoff := time.Now().AddDate(0, 0, -int(days))
	affected, err := model.DeleteCallLogsBefore(cutoff)
	if err != nil {
		return 0, err
	}
	if affected > 0 {
		log.Printf("[maintenance] cleaned %d call logs older than %d days", affected, days)
	}
	return affected, nil
}

// StartMaintenanceTasks 启动后台维护任务(日志 TTL 清理等)。每日执行一次,启动后先等 1 分钟再首次执行。
// ctx 取消时退出。
func StartMaintenanceTasks(ctx context.Context) {
	go func() {
		// 启动后短暂延迟,避免与启动初始化争抢资源
		select {
		case <-time.After(time.Minute):
		case <-ctx.Done():
			return
		}
		runMaintenance()

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				runMaintenance()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func runMaintenance() {
	if _, err := CleanupExpiredLogs(); err != nil {
		log.Printf("[maintenance] cleanup expired logs failed: %v", err)
	}
}
