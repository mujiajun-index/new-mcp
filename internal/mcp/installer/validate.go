package installer

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	cmdRe = regexp.MustCompile(`^[A-Za-z0-9._/@:-]+$`)
	regRe = regexp.MustCompile(`^https?://[A-Za-z0-9._/:%?&=-]+$`)

	// shellMetachars are rejected anywhere in command/args as defense-in-depth.
	// (exec.Command already avoids shell interpretation; this yields friendly errors.)
	shellMetachars = ";|&$`<>{}"
)

// Validate checks command, args, and registry against an allowlist/denylist.
// It only rejects clearly dangerous or malformed input — exec.Command uses arg
// slices with no shell, so this is defense-in-depth plus friendly error messages.
func Validate(req *PrepareReq) error {
	if req == nil {
		return errors.New("empty request")
	}

	cmdBase := filepath.Base(req.Command)
	if cmdBase == "" || cmdBase == "." || cmdBase == string(filepath.Separator) {
		return errors.New("command is required")
	}
	if !cmdRe.MatchString(cmdBase) || strings.ContainsAny(req.Command, shellMetachars) {
		return errors.New("command contains invalid characters")
	}

	for _, a := range req.Args {
		if a == "" {
			continue
		}
		if strings.ContainsAny(a, shellMetachars) || hasControlChars(a) {
			return errors.New("arguments contain invalid characters")
		}
	}

	for _, v := range req.Env {
		if hasControlChars(v) {
			return errors.New("environment values contain invalid characters")
		}
	}

	if req.Registry != "" && !regRe.MatchString(req.Registry) {
		return errors.New("registry URL is invalid")
	}
	return nil
}

func hasControlChars(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
