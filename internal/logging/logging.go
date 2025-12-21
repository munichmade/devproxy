// Package logging provides logging utilities for devproxy using the standard library's slog.
package logging

import (
	"io"
	"log/slog"
	"os"
)

// Level is an alias for slog.Level for convenience.
type Level = slog.Level

// Level constants matching slog levels.
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// ParseLevel parses a string into a Level.
func ParseLevel(s string) Level {
	switch s {
	case "debug", "DEBUG":
		return LevelDebug
	case "info", "INFO":
		return LevelInfo
	case "warn", "WARN", "warning", "WARNING":
		return LevelWarn
	case "error", "ERROR":
		return LevelError
	default:
		return LevelInfo
	}
}

// Setup configures the default slog logger with the specified level and output.
func Setup(level Level, w io.Writer) {
	if w == nil {
		w = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewTextHandler(w, opts)
	slog.SetDefault(slog.New(handler))
}

// SetupFile configures the default logger to write to a file.
func SetupFile(level Level, path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	Setup(level, f)
	return nil
}

// Convenience functions that wrap slog package functions.

// Debug logs at debug level.
func Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

// Info logs at info level.
func Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

// Warn logs at warn level.
func Warn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

// Error logs at error level.
func Error(msg string, args ...any) {
	slog.Error(msg, args...)
}

// Default returns the default slog logger.
func Default() *slog.Logger {
	return slog.Default()
}
