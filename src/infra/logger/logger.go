// Package logger provides structured logging using Go's standard library slog.
//
// Why slog over zap?
// - slog is part of the standard library (Go 1.21+), reducing external dependencies
// - It's the idiomatic choice for new Go projects going forward
// - Performance is comparable to zap for most use cases
// - Easier to integrate with existing Go tooling and testing
// - Built-in support for structured logging with type-safe attributes
//
// Usage:
//
//	log := logger.New(cfg.Log)
//	log.Info("server starting", "port", 8080)
//	log.Error("failed to connect", "error", err)
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"jokefactory/src/infra/config"
)

// New creates a new slog.Logger based on the provided configuration.
// It supports JSON and text output formats, and configurable log levels.
func New(cfg config.LogConfig) *slog.Logger {
	return NewWithWriter(cfg, os.Stdout)
}

// NewWithWriter creates a new logger that writes to the specified writer.
// This is useful for testing or writing logs to files.
func NewWithWriter(cfg config.LogConfig, w io.Writer) *slog.Logger {
	level := parseLevel(cfg.Level)

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug, // Add source info only in debug mode
	}

	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "plain":
		handler = &plainHandler{level: level, w: w}
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default:
		handler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(handler)
}

// parseLevel converts a string log level to slog.Level.
// Defaults to Info if the level is not recognized.
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

// WithRequestID returns a new logger with the request ID added to all log entries.
// Use this in HTTP handlers after extracting the request ID from context.
func WithRequestID(log *slog.Logger, requestID string) *slog.Logger {
	return log.With("request_id", requestID)
}

// WithComponent returns a new logger with a component name added.
// Useful for identifying which part of the application generated the log.
func WithComponent(log *slog.Logger, component string) *slog.Logger {
	return log.With("component", component)
}

// Info is a convenience wrapper for slog.Logger.Info, guarding nil.
func Info(log *slog.Logger, msg string, args ...any) {
	if log == nil {
		return
	}
	log.Info(msg, args...)
}

// Warn is a convenience wrapper for slog.Logger.Warn, guarding nil.
func Warn(log *slog.Logger, msg string, args ...any) {
	if log == nil {
		return
	}
	log.Warn(msg, args...)
}

// Error is a convenience wrapper for slog.Logger.Error, guarding nil.
func Error(log *slog.Logger, msg string, args ...any) {
	if log == nil {
		return
	}
	log.Error(msg, args...)
}

// Debug is a convenience wrapper for slog.Logger.Debug, guarding nil.
func Debug(log *slog.Logger, msg string, args ...any) {
	if log == nil {
		return
	}
	log.Debug(msg, args...)
}

// plainHandler writes only the log message, without structured envelope.
type plainHandler struct {
	level slog.Level
	w     io.Writer
	mu    sync.Mutex
}

func (h *plainHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= h.level
}

func (h *plainHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := fmt.Fprintln(h.w, r.Message)
	return err
}

func (h *plainHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	_ = attrs
	return h
}

func (h *plainHandler) WithGroup(name string) slog.Handler {
	_ = name
	return h
}

