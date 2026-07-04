package installer

import (
	"context"
	"os/exec"
)

// DetectNPX returns the npx path and whether BOTH npx and node resolve on PATH.
// (npx ships with Node; requiring node guards against a lone npx shim.)
func DetectNPX(ctx context.Context) (string, bool) {
	np, err := exec.LookPath("npx")
	if err != nil {
		return "", false
	}
	if _, err := exec.LookPath("node"); err != nil {
		return np, false
	}
	return np, true
}

// DetectUV returns the resolved uvx (preferred) or uv path, and whether found.
func DetectUV(ctx context.Context) (string, bool) {
	if p, err := exec.LookPath("uvx"); err == nil {
		return p, true
	}
	if p, err := exec.LookPath("uv"); err == nil {
		return p, true
	}
	return "", false
}

// DetectPlain resolves an arbitrary command (basename or slash-containing path) via LookPath.
func DetectPlain(ctx context.Context, command string) (string, bool) {
	p, err := exec.LookPath(command)
	if err != nil {
		return "", false
	}
	return p, true
}
