package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"WARN", LevelWarn},
		{"warning", LevelWarn},
		{"WARNING", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"unknown", LevelInfo}, // Default to Info
		{"", LevelInfo},        // Default to Info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseLevel(tt.input); got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetup(t *testing.T) {
	var buf bytes.Buffer
	Setup(LevelInfo, &buf)

	Info("test message", "key", "value")

	output := buf.String()

	// Should contain level
	if !strings.Contains(output, "INFO") {
		t.Errorf("output should contain INFO, got %q", output)
	}

	// Should contain message
	if !strings.Contains(output, "test message") {
		t.Errorf("output should contain message, got %q", output)
	}

	// Should contain key-value pair
	if !strings.Contains(output, "key=value") {
		t.Errorf("output should contain key=value, got %q", output)
	}
}

func TestLevelFiltering(t *testing.T) {
	tests := []struct {
		name      string
		level     Level
		logLevel  Level
		shouldLog bool
	}{
		{"debug at debug level", LevelDebug, LevelDebug, true},
		{"info at debug level", LevelDebug, LevelInfo, true},
		{"warn at debug level", LevelDebug, LevelWarn, true},
		{"error at debug level", LevelDebug, LevelError, true},

		{"debug at info level", LevelInfo, LevelDebug, false},
		{"info at info level", LevelInfo, LevelInfo, true},
		{"warn at info level", LevelInfo, LevelWarn, true},
		{"error at info level", LevelInfo, LevelError, true},

		{"debug at warn level", LevelWarn, LevelDebug, false},
		{"info at warn level", LevelWarn, LevelInfo, false},
		{"warn at warn level", LevelWarn, LevelWarn, true},
		{"error at warn level", LevelWarn, LevelError, true},

		{"debug at error level", LevelError, LevelDebug, false},
		{"info at error level", LevelError, LevelInfo, false},
		{"warn at error level", LevelError, LevelWarn, false},
		{"error at error level", LevelError, LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			Setup(tt.level, &buf)

			switch tt.logLevel {
			case LevelDebug:
				Debug("msg")
			case LevelInfo:
				Info("msg")
			case LevelWarn:
				Warn("msg")
			case LevelError:
				Error("msg")
			}

			hasOutput := buf.Len() > 0
			if hasOutput != tt.shouldLog {
				t.Errorf("expected log output: %v, got output: %v (buf: %q)", tt.shouldLog, hasOutput, buf.String())
			}
		})
	}
}

func TestSetupFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	err := SetupFile(LevelInfo, logPath)
	if err != nil {
		t.Fatalf("SetupFile() error = %v", err)
	}

	Info("file test message")

	// Read the file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "file test message") {
		t.Errorf("log file should contain message, got %q", string(content))
	}
}

func TestSetupFile_InvalidPath(t *testing.T) {
	err := SetupFile(LevelInfo, "/nonexistent/dir/test.log")
	if err == nil {
		t.Error("SetupFile should return error for invalid path")
	}
}

func TestDefault(t *testing.T) {
	logger := Default()
	if logger == nil {
		t.Error("Default() should not return nil")
	}

	// Should be a *slog.Logger
	if _, ok := interface{}(logger).(*slog.Logger); !ok {
		t.Error("Default() should return *slog.Logger")
	}
}

func TestNilOutput(t *testing.T) {
	// Should not panic with nil output (defaults to stdout)
	Setup(LevelInfo, nil)
	// Verify it doesn't panic by logging something
	Info("nil output test")
}
