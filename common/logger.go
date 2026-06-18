package common

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

// LogDir is the directory that log files are written to.
// Configure via the --log-dir CLI flag (default "./logs").
// Pass an empty string (--log-dir "") to disable file logging and keep
// console output only. Mirrors new-api's common.LogDir flag.
var LogDir = flag.String("log-dir", "./logs", "specify the log directory")

// logFile holds the active log file handle, or nil when file logging is off.
var logFile *os.File

// SetupLogger opens a log file under LogDir (when set) and redirects the
// standard library logger and Gin's writers to write to BOTH the console
// (stdout/stderr, used by `docker logs`) and the file (used by the ./logs
// bind mount). Behaves like new-api's logger.SetupLogger, simplified: one
// file per process start, no mid-run rotation.
func SetupLogger() {
	if *LogDir == "" {
		return
	}

	// Resolve to an absolute path and create the directory if it is missing.
	absDir, err := filepath.Abs(*LogDir)
	if err != nil {
		log.Fatalf("failed to resolve log dir %q: %v", *LogDir, err)
	}
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		if err := os.MkdirAll(absDir, 0777); err != nil {
			log.Fatalf("failed to create log dir %q: %v", absDir, err)
		}
	}
	*LogDir = absDir

	// One file per process start, timestamped to avoid clobbering prior logs.
	logPath := filepath.Join(absDir, fmt.Sprintf("newmcp-%s.log", time.Now().Format("20060102150405")))
	fd, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("failed to open log file %q: %v", logPath, err)
	}
	logFile = fd

	// Tee output so logs reach both the container console and the file.
	log.SetOutput(io.MultiWriter(os.Stderr, fd))
	gin.DefaultWriter = io.MultiWriter(os.Stdout, fd)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, fd)

	log.Printf("logging to file: %s", logPath)
}

// CloseLogFile closes the active log file, if one is open. Call on shutdown.
func CloseLogFile() {
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
}
