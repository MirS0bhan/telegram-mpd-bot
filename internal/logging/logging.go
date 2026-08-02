// Package logging provides a small wrapper around log/slog that gives the
// rest of the application structured, multi-level (debug/info/warn/error)
// logging with per-component context.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Config controls how the root logger is built.
type Config struct {
	// Level is one of "debug", "info", "warn"/"warning", "error". Defaults to "info".
	Level string `yaml:"level"`
	// Format is "text" (human friendly) or "json" (machine friendly). Defaults to "text".
	Format string `yaml:"format"`
}

// ParseLevel converts a level name into an slog.Level, defaulting to Info
// when the value is empty or unrecognized.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info", "":
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}

// New builds the root application logger from the given config and writer.
// A nil writer defaults to os.Stderr.
func New(cfg Config, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}

	level := ParseLevel(cfg.Level)
	opts := &slog.HandlerOptions{
		Level: level,
		// Source location is most useful when actively debugging, so only
		// pay its cost when debug logging is enabled.
		AddSource: level <= slog.LevelDebug,
	}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(cfg.Format)) {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	default:
		handler = slog.NewTextHandler(w, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// Component returns a child logger tagged with the given component name so
// log lines can be attributed to the package/subsystem that emitted them.
func Component(logger *slog.Logger, name string) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger.With("component", name)
}

// LevelNames lists the accepted level values, mainly for error messages.
func LevelNames() string {
	return fmt.Sprintf("%s, %s, %s, %s", "debug", "info", "warn", "error")
}
