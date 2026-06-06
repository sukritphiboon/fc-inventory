// Package logging wires slog to stdout + a rotating file (lumberjack).
// Mirrors app.py:22-46: rotating 5 MB x 3 backups, UTF-8, root level INFO,
// urllib3/werkzeug-equivalent quiet-down is unnecessary in Go since we use
// net/http (no third-party HTTP logger).
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/kimzhong/fc-inventory/internal/config"
)

// Setup builds a *slog.Logger that writes to both stderr and the rotating
// log file, then installs it as the process-wide default.
//
// fileSizeMB, maxBackups, maxAgeDays come from the loaded config; level
// is the resolved log level string ("debug"|"info"|"warn"|"error").
func Setup(cfg *config.LoggingConfig) (*slog.Logger, error) {
	if cfg == nil {
		return slog.Default(), nil
	}
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	rotator := &lumberjack.Logger{
		Filename:   cfg.File,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
	}

	// Always write a startup banner to the file so a freshly rotated
	// backup is never empty.
	if _, err := io.WriteString(rotator, "\n"); err != nil {
		return nil, fmt.Errorf("touch log file: %w", err)
	}

	handler := slog.NewTextHandler(io.MultiWriter(os.Stderr, rotator), &slog.HandlerOptions{
		Level: level,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger, nil
}

// parseLevel maps the YAML string to slog.Level. Matches the values
// accepted by config.Validate.
func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("logging.level %q invalid", s)
	}
}
