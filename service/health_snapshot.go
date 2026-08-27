package service

import (
	"sync"
	"time"

	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

// ServiceHealthSnapshot 单个非 stdio 服务的健康聚合结果:近 1h 健康分 +
// 近 24h 每小时一桶的成功/失败/延迟 + 24h 内最近一次错误。全部来自一次
// mcp_call_logs 窄行扫描(真实网关调用,不做主动探测)。
type ServiceHealthSnapshot struct {
	Score            *int            // nil = 近 1h 无调用
	State            string          // 见 health_score.go 档位常量
	Buckets          []dto.HealthBucket // 恒 24 项,无调用桶 Total==0
	LastErrorMessage string
	LastErrorUnix    int64 // 0 = 无
}

// healthCacheTTL 进程内缓存:总览页 5s 轮询,30s 重建一次摊平 DB 压力。
// 回写(health_status)即时生效,分数/时间条滞后 ≤TTL,有意为之。
const healthCacheTTL = 30 * time.Second

var healthCache struct {
	sync.Mutex
	builtAt time.Time
	ids     map[int64]struct{}
	data    map[int64]*ServiceHealthSnapshot
}

// GetServiceHealthSnapshots 返回一批非 stdio 服务的健康快照。缓存命中条件:
// 未过 TTL 且请求的 ID 集合与构建时一致(不同管理员可见服务集不同,集合变了
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

// healthAcc 单服务聚合器:24 桶计数/延迟和 + 近 1h 窗口 + 突发窗口失败数 +
// 最近错误。桶内均值收尾时统一用 durSum/total 求出。
type healthAcc struct {
	bucketTotal  [24]int64
	bucketSucc   [24]int64
	bucketDurSum [24]int64
	winTotal     int64
	winSuccess   int64
	winDurSum    int64
	failures10   int64
	lastErrMsg   string
	lastErrUnix  int64
}

// buildServiceHealthSnapshots 一次窄行扫描聚合全部服务。桶边界为服务器本地
// 整点(time.Truncate 对整小时等价于本地整点截断;半小时时区偏移下桶边界
// 仍是整点 UTC 小时,与 GetCallLogStats 的 Truncate 用法口径一致),24 桶 =
// 23 个完整小时 + 当前未满的小时;扫描起点即首桶起点,不多扫尾巴。
func buildServiceHealthSnapshots(serviceIDs []int64, now time.Time) (map[int64]*ServiceHealthSnapshot, error) {
	firstBucket := now.Truncate(time.Hour).Add(-23 * time.Hour)
	rows, err := model.GetCallLogRowsForServices(serviceIDs, firstBucket)
	if err != nil {
		return nil, err
	}

	accs := make(map[int64]*healthAcc, len(serviceIDs))
	winSince := now.Add(-time.Hour)
	burstSince := now.Add(-healthBurstWindow)
	for i := range rows {
		r := &rows[i]
		acc := accs[r.ServiceID]
		if acc == nil {
			acc = &healthAcc{}
			accs[r.ServiceID] = acc
		}
		idx := int(r.CreatedAt.Sub(firstBucket) / time.Hour)
		if idx < 0 || idx > 23 { // 防御时钟漂移的越界行,正常查询范围不会出现
			continue
		}
		acc.bucketTotal[idx]++
		acc.bucketDurSum[idx] += int64(r.DurationMs)
		success := r.ResponseStatus == "success"
		if success {
			acc.bucketSucc[idx]++
		} else {
			if r.CreatedAt.After(burstSince) {
				acc.failures10++
			}
			// rows 无序,比较时间取真正最近的一次失败
			if r.CreatedAt.Unix() >= acc.lastErrUnix {
				acc.lastErrUnix = r.CreatedAt.Unix()
				acc.lastErrMsg = r.ErrorMessage
			}
		}
		if r.CreatedAt.After(winSince) {
			acc.winTotal++
			acc.winDurSum += int64(r.DurationMs)
			if success {
				acc.winSuccess++
			}
		}
	}

	snaps := make(map[int64]*ServiceHealthSnapshot, len(serviceIDs))
	for _, id := range serviceIDs {
		acc := accs[id] // 无任何调用的服务为 nil,照样产出全灰快照
		snap := &ServiceHealthSnapshot{State: HealthStateNoData}
		snap.Buckets = make([]dto.HealthBucket, 24)
		if acc != nil && acc.winTotal > 0 {
			score, state := ComputeHealthScore(HealthWindowAgg{
				TotalCalls:     acc.winTotal,
				SuccessCalls:   acc.winSuccess,
				AvgDurationMs:  float64(acc.winDurSum) / float64(acc.winTotal),
				FailuresLast10: acc.failures10,
			})
			snap.Score = &score
			snap.State = state
		}
		if acc != nil {
			snap.LastErrorMessage = acc.lastErrMsg
			snap.LastErrorUnix = acc.lastErrUnix
		}
		for i := 0; i < 24; i++ {
			b := &snap.Buckets[i]
			b.StartUnix = firstBucket.Add(time.Duration(i) * time.Hour).Unix()
			if acc != nil {
				b.Total = acc.bucketTotal[i]
				b.Success = acc.bucketSucc[i]
				if b.Total > 0 {
					b.AvgDurationMs = acc.bucketDurSum[i] / b.Total
				}
			}
		}
		snaps[id] = snap
	}
	return snaps, nil
}
