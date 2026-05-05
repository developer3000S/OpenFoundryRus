package observability

import (
	"log/slog"
	"os"
	"strings"
)

// LogFormat controls the slog handler chosen by InitLogging.
type LogFormat string

const (
	LogFormatJSON  LogFormat = "json"
	LogFormatPlain LogFormat = "plain"
)

// InitLogging installs slog as the default logger. Mirrors the
// behaviour of the Rust `init_tracing` helper:
//
//   - LOG_FORMAT=json → JSON handler (production)
//   - otherwise       → text handler (developer-friendly)
//
// The minimum level honours the OF_LOG_LEVEL environment variable
// (debug, info, warn, error). Default is info.
func InitLogging(service, version string) *slog.Logger {
	level := parseLevel(os.Getenv("OF_LOG_LEVEL"))
	format := strings.ToLower(os.Getenv("LOG_FORMAT"))

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch LogFormat(format) {
	case LogFormatJSON:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	logger := slog.New(handler).With(
		slog.String("service", service),
		slog.String("version", version),
	)
	slog.SetDefault(logger)
	logger.Info("observability initialized",
		slog.String("log_format", string(format)),
	)
	return logger
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
