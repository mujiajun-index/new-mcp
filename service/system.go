package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/common"
)

const (
	// githubRepo 为查询最新 release 的 GitHub 仓库（owner/repo）。
	githubRepo = "mujiajun-index/new-mcp"
	// githubCacheTTL 限制重复点击时实际访问 GitHub 的频率。
	githubCacheTTL = 5 * time.Minute
	// githubTimeout 为单次请求 GitHub 的超时时间。
	githubTimeout = 10 * time.Second
)

// SystemService 封装只读的系统信息与 GitHub 更新检查。
// 零值可用，由 controller 持有包级单例。
type SystemService struct{}

// SystemInfo 为 GetSystemInfo 的返回结构。
type SystemInfo struct {
	Version   string `json:"version"`
	StartTime int64  `json:"start_time"`
}

// GetSystemInfo 返回当前版本与启动时间。
func (s *SystemService) GetSystemInfo() SystemInfo {
	return SystemInfo{
		Version:   common.Version,
		StartTime: common.StartTime,
	}
}

// ReleaseInfo 是 GitHub /repos/{owner}/{repo}/releases/latest 响应中我们关心的子集。
type ReleaseInfo struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// CheckUpdateResult 为 CheckUpdate 的返回结构。
//
//   - HasRelease == false：仓库尚无任何 release（GitHub 返回 404），
//     Release 为 nil，controller 仍返回 success，前端显示「暂未发布任何版本」。
//   - HasRelease == true：Release 已填充最新版本信息。
type CheckUpdateResult struct {
	HasRelease bool         `json:"has_release"`
	Release    *ReleaseInfo `json:"release,omitempty"`
}

// --- 内存 TTL 缓存 ---------------------------------------------------------

type cachedRelease struct {
	at       time.Time
	result   *CheckUpdateResult
	fetchErr error // 缓存错误，避免 GitHub 宕/限流时被连点刷爆额度
}

var (
	githubCacheMu  sync.Mutex
	githubCacheVal cachedRelease
)

// CheckUpdate 获取 GitHub 最新 release，带 5 分钟 TTL 缓存。
// 成功与失败都会被缓存：缓存有效期内（包括缓存的错误）直接返回，不再访问 GitHub。
func (s *SystemService) CheckUpdate() (*CheckUpdateResult, error) {
	githubCacheMu.Lock()
	defer githubCacheMu.Unlock()

	if githubCacheVal.result != nil && time.Since(githubCacheVal.at) < githubCacheTTL {
		if githubCacheVal.fetchErr != nil {
			return nil, githubCacheVal.fetchErr
		}
		return githubCacheVal.result, nil
	}

	result, err := fetchLatestRelease()
	githubCacheVal = cachedRelease{
		at:       time.Now(),
		result:   result,
		fetchErr: err,
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// fetchLatestRelease 执行实际的（未缓存）GitHub 请求。
func fetchLatestRelease() (*CheckUpdateResult, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)

	ctx, cancel := context.WithTimeout(context.Background(), githubTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "newmcp-admin-dashboard")

	client := &http.Client{Timeout: githubTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub 失败: %w", err)
	}
	defer resp.Body.Close()

	// 关键边界：仓库尚无 release 时 GitHub 返回 404。
	// 必须在反序列化之前拦截，避免把 {"message":"Not Found",...} 当成空 release。
	if resp.StatusCode == http.StatusNotFound {
		return &CheckUpdateResult{HasRelease: false, Release: nil}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回状态码 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var rel ReleaseInfo
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if rel.TagName == "" {
		// 防御性：返回 200 但缺少 tag_name，视为无 release。
		return &CheckUpdateResult{HasRelease: false, Release: nil}, nil
	}
	return &CheckUpdateResult{HasRelease: true, Release: &rel}, nil
}
