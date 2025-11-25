package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Logger wraps slog.Logger with additional functionality
type Logger struct {
	*slog.Logger
}

// Config holds logger configuration
type Config struct {
	Level  string // debug, info, warn, error
	Format string // json, text
	Output io.Writer
}

// New creates a new logger instance
func New(config Config) *Logger {
	// Parse log level
	level := parseLevel(config.Level)

	// Determine output
	output := config.Output
	if output == nil {
		output = os.Stdout
	}

	// Create handler based on format
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
		AddSource: level == slog.LevelDebug, // Add source location for debug level
	}

	switch strings.ToLower(config.Format) {
	case "text":
		handler = slog.NewTextHandler(output, opts)
	default: // json is default
		handler = slog.NewJSONHandler(output, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
	}
}

// parseLevel converts string level to slog.Level
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithContext creates a new logger with context fields
func (l *Logger) WithContext(args ...any) *Logger {
	return &Logger{
		Logger: l.Logger.With(args...),
	}
}

// WithComponent creates a new logger with a component field
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.Logger.With("component", component),
	}
}

// Default creates a default logger for quick setup
func Default() *Logger {
	return New(Config{
		Level:  "info",
		Format: "json",
		Output: os.Stdout,
	})
}
