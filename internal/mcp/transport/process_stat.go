package transport

import (
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// ProcessTreeStat 汇总 stdio 子进程及其全部后代的资源占用。stdio 服务常用
// npx/uvx 之类 wrapper 启动,真正的内存在 wrapper 拉起的 node/python 子进程里,
// 因此内存/CPU 均按整棵进程树聚合。
type ProcessTreeStat struct {
	Running       bool
	PID           int    // 主(wrapper)进程 PID
	Command       string // 完整命令行
	ProcessCount  int    // 树内进程总数(含主进程)
	RSSBytes      uint64 // 树内 RSS 物理内存总和
	VMSBytes      uint64 // 树内虚拟内存总和
	CPUPercent    float64 // 树累计 CPU 时间 / 主进程生存期,>100% 表示占多核
	UptimeSeconds int64  // 主进程已运行秒数
}

// CollectProcessTreeStat 以 rootPID 为根枚举系统进程构建 ppid→children 索引,
// BFS 收集整棵进程树并汇总内存/CPU/运行时长。主进程已退出(或 PID 复用)时
// 返回 Running:false。逐次现场采集,不做缓存——5s 轮询的单详情页场景下,
// 百级进程的全量枚举开销可接受。
func CollectProcessTreeStat(rootPID int, command string) *ProcessTreeStat {
	stat := &ProcessTreeStat{PID: rootPID, Command: command}

	pids, err := process.Pids()
	if err != nil {
		return stat
	}

	// 一趟扫描建索引;个别进程查询失败(正在退出/权限)直接跳过,不影响其余。
	children := make(map[int32][]int32, len(pids))
	procs := make(map[int32]*process.Process, len(pids))
	for _, pid := range pids {
		p, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		ppid, err := p.Ppid()
		if err != nil {
			continue
		}
		procs[pid] = p
		children[ppid] = append(children[ppid], pid)
	}

	if _, ok := procs[int32(rootPID)]; !ok {
		return stat
	}

	var totalCPUSeconds float64
	var rootCreated int64
	queue := []int32{int32(rootPID)}
	seen := map[int32]bool{int32(rootPID): true}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		p := procs[pid]

		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			stat.RSSBytes += mi.RSS
			stat.VMSBytes += mi.VMS
		}
		// 只取 User+System:gopsutil 在 Linux 上把子进程累计 CPU(cutime/cstime)
		// 映射进 Iowait/Irq 字段,取 Total() 会让树聚合重复计数。
		if t, err := p.Times(); err == nil {
			totalCPUSeconds += t.User + t.System
		}
		if created, err := p.CreateTime(); err == nil && pid == int32(rootPID) {
			rootCreated = created
		}

		stat.ProcessCount++
		for _, child := range children[pid] {
			if !seen[child] {
				seen[child] = true
				queue = append(queue, child)
			}
		}
	}

	// CPU 采用无状态的平均口径:树累计 CPU 时间 / 主进程生存期。gopsutil 的
	// Percent(0) 依赖同一 Process 对象的上次采样,逐请求新建对象时恒为 0,不可用。
	var uptime float64
	if rootCreated > 0 {
		uptime = time.Since(time.UnixMilli(rootCreated)).Seconds()
		if uptime < 1 {
			uptime = 1 // 刚启动时避免除零/放大
		}
		stat.UptimeSeconds = int64(uptime)
	}
	if uptime > 0 {
		stat.CPUPercent = totalCPUSeconds / uptime * 100
	}

	stat.Running = true
	return stat
}
