package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNew_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	log := New(Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	if log == nil {
		t.Fatal("New() returned nil")
	}

	log.Info("test message", "key", "value")

	output := buf.String()
	if output == "" {
		t.Error("Expected log output, got empty string")
	}

	// Verify it's valid JSON
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Errorf("Log output is not valid JSON: %v", err)
	}

	// Check for expected fields
	if logEntry["msg"] != "test message" {
		t.Errorf("Expected msg 'test message', got %v", logEntry["msg"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("Expected key 'value', got %v", logEntry["key"])
	}
}

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	log := New(Config{
		Level:  "info",
		Format: "text",
		Output: &buf,
	})

	if log == nil {
		t.Fatal("New() returned nil")
	}

	log.Info("test message", "key", "value")

	output := buf.String()
	if output == "" {
		t.Error("Expected log output, got empty string")
	}

	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Expected output to contain 'key=value', got: %s", output)
	}
}

func TestParseLevel_Debug(t *testing.T) {
	level := parseLevel("debug")
	if level != slog.LevelDebug {
		t.Errorf("Expected LevelDebug, got %v", level)
	}
}

func TestParseLevel_Info(t *testing.T) {
	level := parseLevel("info")
	if level != slog.LevelInfo {
		t.Errorf("Expected LevelInfo, got %v", level)
	}
}

func TestParseLevel_Warn(t *testing.T) {
	level := parseLevel("warn")
	if level != slog.LevelWarn {
		t.Errorf("Expected LevelWarn, got %v", level)
	}

	// Test "warning" alias
	level = parseLevel("warning")
	if level != slog.LevelWarn {
		t.Errorf("Expected LevelWarn for 'warning', got %v", level)
	}
}

func TestParseLevel_Error(t *testing.T) {
	level := parseLevel("error")
	if level != slog.LevelError {
		t.Errorf("Expected LevelError, got %v", level)
	}
}

func TestParseLevel_Invalid(t *testing.T) {
	level := parseLevel("invalid")
	if level != slog.LevelInfo {
		t.Errorf("Expected default LevelInfo for invalid level, got %v", level)
	}
}

func TestParseLevel_CaseInsensitive(t *testing.T) {
	testCases := []struct {
		input    string
		expected slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"Info", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"ErRoR", slog.LevelError},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			level := parseLevel(tc.input)
			if level != tc.expected {
				t.Errorf("Expected %v for input %s, got %v", tc.expected, tc.input, level)
			}
		})
	}
}

func TestLogger_DebugLevel_ShowsDebug(t *testing.T) {
	var buf bytes.Buffer
	log := New(Config{
		Level:  "debug",
		Format: "json",
		Output: &buf,
	})

	log.Debug("debug message")

	output := buf.String()
	if output == "" {
		t.Error("Expected debug log output at debug level")
	}
}

func TestLogger_InfoLevel_HidesDebug(t *testing.T) {
	var buf bytes.Buffer
	log := New(Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	log.Debug("debug message")

	output := buf.String()
	if output != "" {
		t.Error("Expected no debug log output at info level")
	}
}

func TestLogger_ErrorLevel_ShowsError(t *testing.T) {
	var buf bytes.Buffer
	log := New(Config{
		Level:  "error",
		Format: "json",
		Output: &buf,
	})

	log.Error("error message")

	output := buf.String()
	if output == "" {
		t.Error("Expected error log output at error level")
	}

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Errorf("Log output is not valid JSON: %v", err)
	}

	if logEntry["level"] != "ERROR" {
		t.Errorf("Expected level ERROR, got %v", logEntry["level"])
	}
}

func TestLogger_ErrorLevel_HidesInfo(t *testing.T) {
	var buf bytes.Buffer
	log := New(Config{
		Level:  "error",
		Format: "json",
		Output: &buf,
	})

	log.Info("info message")

	output := buf.String()
	if output != "" {
		t.Error("Expected no info log output at error level")
	}
}

func TestLogger_WithContext(t *testing.T) {
	var buf bytes.Buffer
	log := New(Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	contextLog := log.WithContext("request_id", "12345", "user", "test")
	contextLog.Info("test message")

	output := buf.String()
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Errorf("Log output is not valid JSON: %v", err)
	}

	if logEntry["request_id"] != "12345" {
		t.Errorf("Expected request_id 12345, got %v", logEntry["request_id"])
	}
	if logEntry["user"] != "test" {
		t.Errorf("Expected user test, got %v", logEntry["user"])
	}
}

func TestLogger_WithComponent(t *testing.T) {
	var buf bytes.Buffer
	log := New(Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	componentLog := log.WithComponent("database")
	componentLog.Info("test message")

	output := buf.String()
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Errorf("Log output is not valid JSON: %v", err)
	}

	if logEntry["component"] != "database" {
		t.Errorf("Expected component database, got %v", logEntry["component"])
	}
}

func TestLogger_Default(t *testing.T) {
	log := Default()

	if log == nil {
		t.Fatal("Default() returned nil")
	}

	// Default should create a logger that doesn't panic
	log.Info("test message")
}

func TestNew_NilOutput_UsesStdout(t *testing.T) {
	log := New(Config{
		Level:  "info",
		Format: "json",
		Output: nil, // Should default to os.Stdout
	})

	if log == nil {
		t.Fatal("New() returned nil with nil output")
	}

	// Should not panic
	log.Info("test message")
}

func TestLogger_ChainedContext(t *testing.T) {
	var buf bytes.Buffer
	log := New(Config{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	// Chain multiple context additions
	log1 := log.WithContext("key1", "value1")
	log2 := log1.WithContext("key2", "value2")
	log3 := log2.WithComponent("test-component")

	log3.Info("test message")

	output := buf.String()
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Errorf("Log output is not valid JSON: %v", err)
	}

	if logEntry["key1"] != "value1" {
		t.Errorf("Expected key1 value1, got %v", logEntry["key1"])
	}
	if logEntry["key2"] != "value2" {
		t.Errorf("Expected key2 value2, got %v", logEntry["key2"])
	}
	if logEntry["component"] != "test-component" {
		t.Errorf("Expected component test-component, got %v", logEntry["component"])
	}
}
