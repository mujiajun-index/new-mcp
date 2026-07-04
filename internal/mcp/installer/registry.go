package installer

import (
	"context"
	"strings"
	"time"
)

// RegistryPreset is a named package-registry mirror option.
type RegistryPreset struct {
	Key   string
	Label string
	URL   string
}

// NPMRegistryPresets are the mirrors offered for the npx branch.
var NPMRegistryPresets = []RegistryPreset{
	{Key: "npmmirror", Label: "淘宝", URL: "https://registry.npmmirror.com"},
}

// UVRegistryPresets are the mirrors offered for the uvx branch.
var UVRegistryPresets = []RegistryPreset{
	{Key: "tsinghua", Label: "清华", URL: "https://pypi.tuna.tsinghua.edu.cn/simple"},
	{Key: "aliyun", Label: "阿里云", URL: "http://mirrors.aliyun.com/pypi/simple/"},
	{Key: "ustc", Label: "USTC", URL: "https://mirrors.ustc.edu.cn/pypi/simple/"},
	{Key: "huaweicloud", Label: "华为云", URL: "https://repo.huaweicloud.com/repository/pypi/simple/"},
	{Key: "tencent", Label: "腾讯云", URL: "https://mirrors.cloud.tencent.com/pypi/simple/"},
}

// RegistryEnv returns the env-var map to inject for a branch + mirror URL.
// An empty registryURL yields an empty map (use system defaults).
//
//	npx → {NPM_CONFIG_REGISTRY}
//	uvx → {UV_DEFAULT_INDEX, PIP_INDEX_URL}   (mirror Cherry Studio)
//	plain → {}
func RegistryEnv(branch Branch, registryURL string) map[string]string {
	if registryURL == "" {
		return map[string]string{}
	}
	switch branch {
	case BranchNPX:
		return map[string]string{"NPM_CONFIG_REGISTRY": registryURL}
	case BranchUVX:
		return map[string]string{"UV_DEFAULT_INDEX": registryURL, "PIP_INDEX_URL": registryURL}
	default:
		return map[string]string{}
	}
}

// ExtractPackageName returns the first non-flag argument (tokens starting with
// "-" are skipped; "--" terminates flag parsing but is itself skipped).
//   ["-y", "@modelcontextprotocol/server-memory"] → "@modelcontextprotocol/server-memory"
//   ["mcp-server-time"]                          → "mcp-server-time"
func ExtractPackageName(args []string) string {
	for _, a := range args {
		if a == "--" {
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a
	}
	return ""
}

// VerifyNpxPackage runs `npm view <pkg> version` with the registry env merged in.
// Exit 0 means the package resolves from the chosen registry.
func VerifyNpxPackage(ctx context.Context, pkg string, regEnv map[string]string) (string, string, error) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return runCaptured(cctx, "npm", []string{"view", pkg, "version"}, regEnv)
}

// InstallUvxTool runs `uv tool install <pkg>` (idempotent) using the absolute
// uvPath so it works even when uv was just installed and is not yet on PATH.
func InstallUvxTool(ctx context.Context, uvPath, pkg string, regEnv map[string]string) (string, string, error) {
	cctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	return runCaptured(cctx, uvPath, []string{"tool", "install", pkg}, regEnv)
}
