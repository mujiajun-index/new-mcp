package installer

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Official uv standalone installers (hardcoded constants — no user input is
// interpolated into the shell string, so this is a safe exception to "no shell").
const (
	uvInstallScriptURL = "https://astral.sh/uv/install.sh"
	uvInstallPSURL     = "https://astral.sh/uv/install.ps1"
)

// EnsureUV installs uv when missing and returns its absolute path.
// Strategy: (1) pip install uv — no shell pipe; (2) official standalone
// installer; (3) resolve absolute path from known install locations (LookPath
// cannot see PATH changes made within this process).
func EnsureUV(ctx context.Context) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	if err := tryPipInstallUV(cctx); err == nil {
		if p := resolveUVPath(); p != "" {
			return p, nil
		}
	}
	if err := runUVInstaller(cctx); err == nil {
		if p := resolveUVPath(); p != "" {
			return p, nil
		}
	}
	return "", errors.New("could not install uv automatically; please install uv manually (https://docs.astral.sh/uv/getting-started/installation/)")
}

func tryPipInstallUV(ctx context.Context) error {
	for _, pip := range []string{"pip", "pip3"} {
		if _, err := exec.LookPath(pip); err != nil {
			continue
		}
		_, _, err := runCaptured(ctx, pip, []string{"install", "uv"}, nil)
		if err == nil {
			return nil
		}
	}
	return errors.New("pip not available or pip install uv failed")
}

// runUVInstaller invokes the official standalone installer. The URL is a
// hardcoded constant; no user input is part of the shell command.
func runUVInstaller(ctx context.Context) error {
	if runtime.GOOS == "windows" {
		_, _, err := runCaptured(ctx, "powershell.exe",
			[]string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "irm " + uvInstallPSURL + " | iex"}, nil)
		return err
	}
	_, _, err := runCaptured(ctx, "sh", []string{"-c", "curl -LsSf " + uvInstallScriptURL + " | sh"}, nil)
	return err
}

// resolveUVPath finds uv by PATH first, then by probing known install locations
// (necessary because a just-installed uv may not be visible to LookPath in this process).
func resolveUVPath() string {
	if p, err := exec.LookPath("uvx"); err == nil {
		return p
	}
	if p, err := exec.LookPath("uv"); err == nil {
		return p
	}
	for _, c := range uvCandidateLocations() {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

func uvCandidateLocations() []string {
	home, _ := os.UserHomeDir()
	var candidates []string
	if runtime.GOOS == "windows" {
		if p := os.Getenv("USERPROFILE"); p != "" {
			candidates = append(candidates, filepath.Join(p, ".local", "bin", "uv.exe"))
		}
		if p := os.Getenv("LOCALAPPDATA"); p != "" {
			candidates = append(candidates, filepath.Join(p, "uv", "uv.exe"))
		}
	} else if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".local", "bin", "uv"),
			filepath.Join(home, ".cargo", "bin", "uv"),
		)
	}
	return candidates
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
