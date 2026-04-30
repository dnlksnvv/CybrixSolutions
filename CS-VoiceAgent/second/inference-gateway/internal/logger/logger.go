// Package logger configures the process-wide structured logger.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a JSON slog.Logger with level driven by LOG_LEVEL env.
func New() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(h)
}
