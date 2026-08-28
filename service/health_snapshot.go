package service

import (
	"sync"
	"time"

	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

// 近期调用窗口口径(对齐 CLIProxyAPI recent_requests):20 桶 × 10 分钟 = 近 200
// 分钟滚动窗口。桶边界为绝对 10 分钟整点(epoch/600 对齐),纯被动——只聚合
// mcp_call_logs 真实调用(含手动测试),不做主动探测。
const (
	healthBucketCount  = 20
	healthBucketLength = 10 * time.Minute
)

// ServiceHealthSnapshot 单个非 stdio 服务的近期调用聚合:固定 20 桶成败计数 +
// 窗口内最近一次调用 + 窗口内最近一次错误。全部来自一次 mcp_call_logs 窄行扫描;
// 健康状态本身不在此判定,由调用方结合连接状态实时推导(见 derivePassiveHealth)。
type ServiceHealthSnapshot struct {
	Buckets          []dto.HealthBucket // 恒 20 项(旧→新),无调用桶 Success+Failed==0
	LastCallUnix     int64              // 窗口内最近一次调用,0 = 窗口内无调用
	LastErrorMessage string
	LastErrorUnix    int64 // 0 = 无
}

// healthCacheTTL 进程内缓存:总览页 5s 轮询,30s 重建一次摊平 DB 压力。
const healthCacheTTL = 30 * time.Second

var healthCache struct {
	sync.Mutex
	builtAt time.Time
	ids     map[int64]struct{}
	data    map[int64]*ServiceHealthSnapshot
}

// GetServiceHealthSnapshots 返回一批非 stdio 服务的近期调用快照。缓存命中条件:
// 未过 TTL 且请求的 ID 集合与构建时一致(不同用户可见服务集不同,集合变了
// 直接重建)。查询出错返回 nil,调用方降级为不带健康字段(总览不因此报错)。
func GetServiceHealthSnapshots(serviceIDs []int64) map[int64]*ServiceHealthSnapshot {
	if len(serviceIDs) == 0 {
		return nil
	}
	now := time.Now()

	healthCache.Lock()
	defer healthCache.Unlock()

	if healthCache.data != nil && now.Sub(healthCache.builtAt) < healthCacheTTL &&
		sameIDSets(healthCache.ids, serviceIDs) {
		return healthCache.data
	}
	snaps, err := buildServiceHealthSnapshots(serviceIDs, now)
	if err != nil {
		return nil
	}
	ids := make(map[int64]struct{}, len(serviceIDs))
	for _, id := range serviceIDs {
		ids[id] = struct{}{}
	}
	healthCache.builtAt = now
	healthCache.ids = ids
	healthCache.data = snaps
	return snaps
}

func sameIDSets(cached map[int64]struct{}, want []int64) bool {
	if len(cached) != len(want) {
		return false
	}
	for _, id := range want {
		if _, ok := cached[id]; !ok {
			return false
		}
	}
	return true
}

// buildServiceHealthSnapshots 一次窄行扫描聚合全部服务。桶边界为绝对 10 分钟
// 整点;分桶在 Go 侧完成,避免 SQLite/MySQL/PG 三方言的时间表达式分叉(与
// GetCallLogRowsForServices 的既有取舍一致)。
func buildServiceHealthSnapshots(serviceIDs []int64, now time.Time) (map[int64]*ServiceHealthSnapshot, error) {
	firstBucket := now.Truncate(healthBucketLength).Add(-(healthBucketCount - 1) * healthBucketLength)
	rows, err := model.GetCallLogRowsForServices(serviceIDs, firstBucket)
	if err != nil {
		return nil, err
	}

	type healthAcc struct {
		bucketSucc [healthBucketCount]int64
		bucketFail [healthBucketCount]int64
		lastCall   int64
		lastErrMsg string
		lastErrAt  int64
	}
	accs := make(map[int64]*healthAcc, len(serviceIDs))
	for i := range rows {
		r := &rows[i]
		acc := accs[r.ServiceID]
		if acc == nil {
			acc = &healthAcc{}
			accs[r.ServiceID] = acc
		}
		idx := int(r.CreatedAt.Sub(firstBucket) / healthBucketLength)
		if idx < 0 || idx >= healthBucketCount { // 防御时钟漂移的越界行,正常查询范围不会出现
			continue
		}
		if r.ResponseStatus == "success" {
			acc.bucketSucc[idx]++
		} else {
			acc.bucketFail[idx]++
			// rows 无序,比较时间取真正最近的一次失败
			if r.CreatedAt.Unix() >= acc.lastErrAt {
				acc.lastErrAt = r.CreatedAt.Unix()
				acc.lastErrMsg = r.ErrorMessage
			}
		}
		if r.CreatedAt.Unix() > acc.lastCall {
			acc.lastCall = r.CreatedAt.Unix()
		}
	}

	snaps := make(map[int64]*ServiceHealthSnapshot, len(serviceIDs))
	for _, id := range serviceIDs {
		acc := accs[id] // 无任何调用的服务为 nil,照样产出全灰快照
		snap := &ServiceHealthSnapshot{Buckets: make([]dto.HealthBucket, healthBucketCount)}
		for i := 0; i < healthBucketCount; i++ {
			snap.Buckets[i].StartUnix = firstBucket.Add(time.Duration(i) * healthBucketLength).Unix()
			if acc != nil {
				snap.Buckets[i].Success = acc.bucketSucc[i]
				snap.Buckets[i].Failed = acc.bucketFail[i]
			}
		}
		if acc != nil {
			snap.LastCallUnix = acc.lastCall
			snap.LastErrorMessage = acc.lastErrMsg
			snap.LastErrorUnix = acc.lastErrAt
		}
		snaps[id] = snap
	}
	return snaps, nil
}
