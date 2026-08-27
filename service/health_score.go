package service

import "time"

// 非 Stdio 服务健康分:灵感来自 CliRelay README 描述的 0-100 健康分(成功率/延迟/
// 错误模式加权;该参考项目实际未实现,此处为原创公式)。阈值集中在此,前端分档
// 颜色(web/src/features/services/components/service-health-bar.tsx)与此对齐。
const (
	healthLatencyGoodMs = 1000.0  // 1h 窗口均延迟 ≤1s → 延迟分满分
	healthLatencyBadMs  = 20000.0 // ≥20s → 0 分
	healthBurstWindow   = 10 * time.Minute
	healthBurstFailures = 3  // 突发窗口内失败 ≥3 → 触发硬封顶(错误模式惩罚)
	healthBurstScoreCap = 40 // 封顶分,恒落 critical 档
	weightSuccessRate   = 0.7
	weightLatency       = 0.3
)

// 健康档位(health_state),前端按此选色:healthy→emerald / ok→green /
// degraded→amber / critical→red / no_data→灰。
const (
	HealthStateHealthy   = "healthy"   // ≥90
	HealthStateOk        = "ok"        // ≥70
	HealthStateDegraded  = "degraded"  // ≥50
	HealthStateCritical  = "critical"  // <50
	HealthStateNoData    = "no_data"   // 窗口内无调用
)

// HealthWindowAgg 健康分输入:近 1h 聚合 + 突发窗口失败数,由健康快照的同一份
// 24h 窄行扫描得出(见 health_snapshot.go),纯函数便于表驱动单测。
type HealthWindowAgg struct {
	TotalCalls     int64
	SuccessCalls   int64
	AvgDurationMs  float64 // 1h 窗口内全部调用(成功+失败)的均延迟
	FailuresLast10 int64   // 近 10 分钟失败数
}

// ComputeHealthScore 计算 0-100 健康分与档位:
//
//	score = round(100 × (0.7×成功率 + 0.3×延迟分))
//	延迟分:均延迟 ≤1s 为 1,≥20s 为 0,之间线性
//	错误模式惩罚:近 10 分钟失败 ≥3 → score 封顶 40(即使成功率 100%)
//	TotalCalls==0 → (0, no_data)
func ComputeHealthScore(a HealthWindowAgg) (int, string) {
	if a.TotalCalls == 0 {
		return 0, HealthStateNoData
	}
	s := float64(a.SuccessCalls) / float64(a.TotalCalls)

	var l float64
	switch {
	case a.AvgDurationMs <= healthLatencyGoodMs:
		l = 1.0
	case a.AvgDurationMs >= healthLatencyBadMs:
		l = 0.0
	default:
		l = 1 - (a.AvgDurationMs-healthLatencyGoodMs)/(healthLatencyBadMs-healthLatencyGoodMs)
	}

	score := int(float64(100)*(weightSuccessRate*s+weightLatency*l) + 0.5) // 四舍五入
	if a.FailuresLast10 >= healthBurstFailures && score > healthBurstScoreCap {
		score = healthBurstScoreCap
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	switch {
	case score >= 90:
		return score, HealthStateHealthy
	case score >= 70:
		return score, HealthStateOk
	case score >= 50:
		return score, HealthStateDegraded
	default:
		return score, HealthStateCritical
	}
}
