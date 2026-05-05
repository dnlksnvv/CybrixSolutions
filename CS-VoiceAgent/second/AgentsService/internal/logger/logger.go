package logger

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// New создаёт JSON-логгер для сервисов (подходящий для агрегаторов логов).
// Уровень логирования задаётся строкой: trace/debug/info/warn/error/fatal/panic.
func New(level string) zerolog.Logger {
	lvl := parseLevel(level)
	return zerolog.New(os.Stdout).Level(lvl).With().Timestamp().Logger()
}

func parseLevel(s string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info", "":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

