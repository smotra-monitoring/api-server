package testutil

import (
	"bytes"

	"github.com/smotra-monitoring/server/internal/logger"
)

// NewTestLogger creates a logger for testing that writes to a buffer
func NewTestLogger() (*logger.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	log := logger.New(logger.Config{
		Level:  "debug",
		Format: "json",
		Output: &buf,
	})
	return log, &buf
}

// NewSilentLogger creates a logger that discards output
func NewSilentLogger() *logger.Logger {
	return logger.New(logger.Config{
		Level:  "error",
		Format: "json",
		Output: &bytes.Buffer{},
	})
}
