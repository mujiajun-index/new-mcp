// Package installer handles pre-flight detection and installation of stdio
// MCP server runtimes (npx/uvx) and their packages, plus registry/mirror
// injection. It depends only on the standard library so it can be imported
// by bridge/ without creating a cycle.
package installer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxOut = 4096

// Branch classifies the runner command.
type Branch string

const (
	BranchNPX   Branch = "npx"
	BranchUVX   Branch = "uvx"
	BranchPlain Branch = "plain"
)

// PrepareReq mirrors the fields that will later be stored in the service config.
type PrepareReq struct {
	Command  string
	Args     []string
	Env      map[string]string
	Registry string // mirror URL; "" = use system default
}

// PrepareResult is the outcome. Installed is the single gate used by the UI.
type PrepareResult struct {
	Branch       Branch
	RuntimeFound bool
	RuntimePath  string
	DidInstall   bool
	Installed    bool              // THE gate
	PackageName  string
	RegistryEnv  map[string]string // env vars that were/will be injected
	Stdout       string
	Stderr       string
	DurationMs   int64
	Message      string
}

// Prepare runs the full detect → (install uv) → (prefetch/install pkg) pipeline.
// It never returns an error for an "not installed" outcome — that is conveyed
// via PrepareResult.Installed. Errors are reserved for invalid input.
func Prepare(ctx context.Context, req *PrepareReq) (*PrepareResult, error) {
	if err := Validate(req); err != nil {
		return nil, err
	}

	start := time.Now()
	branch := ClassifyCommand(req.Command)
	res := &PrepareResult{
		Branch:      branch,
		RegistryEnv: RegistryEnv(branch, req.Registry),
		PackageName: ExtractPackageName(req.Args),
	}
	defer func() { res.DurationMs = time.Since(start).Milliseconds() }()

	switch branch {
	case BranchNPX:
		prepareNPX(ctx, res)
	case BranchUVX:
		prepareUVX(ctx, res)
	default:
		preparePlain(ctx, req, res)
	}
	return res, nil
}

// ClassifyCommand maps a command (basename or path) to a Branch.
func ClassifyCommand(command string) Branch {
	switch strings.ToLower(filepath.Base(command)) {
	case "npx", "npx.cmd", "npx.exe":
		return BranchNPX
	case "uvx", "uvx.exe":
		return BranchUVX
	}
	return BranchPlain
}

func prepareNPX(ctx context.Context, res *PrepareResult) {
	npxPath, found := DetectNPX(ctx)
	res.RuntimeFound = found
	res.RuntimePath = npxPath
	if !found {
		res.Message = "npx/node not found; please install Node.js first"
		return
	}
	if res.PackageName == "" {
		res.Message = "could not determine the package name from arguments"
		return
	}
	stdout, stderr, err := VerifyNpxPackage(ctx, res.PackageName, res.RegistryEnv)
	res.Stdout, res.Stderr = stdout, stderr
	if err != nil {
		res.Message = fmt.Sprintf("package %s not found in the selected registry: %s", res.PackageName, firstLine(stderr))
		return
	}
	res.DidInstall = true
	res.Installed = true
	res.Message = fmt.Sprintf("package %s resolved", res.PackageName)
}

func prepareUVX(ctx context.Context, res *PrepareResult) {
	uvPath, found := DetectUV(ctx)
	if !found {
		var err error
		uvPath, err = EnsureUV(ctx)
		if err != nil {
			res.Message = fmt.Sprintf("uv auto-install failed: %s", err.Error())
			return
		}
		res.DidInstall = true
	}
	res.RuntimeFound = true
	res.RuntimePath = uvPath
	if res.PackageName == "" {
		res.Message = "could not determine the package name from arguments"
		return
	}
	stdout, stderr, err := InstallUvxTool(ctx, uvPath, res.PackageName, res.RegistryEnv)
	res.Stdout, res.Stderr = stdout, stderr
	if err != nil {
		res.Message = fmt.Sprintf("install %s failed: %s", res.PackageName, firstLine(stderr))
		return
	}
	res.DidInstall = true
	res.Installed = true
	res.Message = fmt.Sprintf("package %s installed", res.PackageName)
}

func preparePlain(ctx context.Context, req *PrepareReq, res *PrepareResult) {
	p, found := DetectPlain(ctx, req.Command)
	res.RuntimeFound = found
	res.RuntimePath = p
	res.Installed = found
	if found {
		res.Message = fmt.Sprintf("command resolved at %s", p)
	} else {
		res.Message = fmt.Sprintf("command %s not found on PATH", req.Command)
	}
}

// --- shared exec helpers ---

// runCaptured runs a command (arg slice — no shell) with extra env merged onto
// os.Environ(), returning truncated stdout/stderr.
func runCaptured(ctx context.Context, name string, args []string, extraEnv map[string]string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), envToSlice(extraEnv)...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return trunc(out.String()), trunc(errb.String()), err
}

func envToSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func trunc(s string) string {
	if len(s) > maxOut {
		return s[:maxOut] + "..."
	}
	return s
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
