package core

import (
	"context"
	"log/slog"
)

// Logger is the hook Payvand uses to report outgoing calls. It is deliberately
// tiny so any logging library can satisfy it without an adapter package; the
// standard library is covered by [SlogLogger].
type Logger interface {
	// Debug reports a normal event, e.g. an outgoing gateway call.
	Debug(ctx context.Context, msg string, fields map[string]string)
	// Error reports a failed event together with the causing error.
	Error(ctx context.Context, msg string, err error, fields map[string]string)
}

// NopLogger is the default logger: it drops everything.
type NopLogger struct{}

// Debug implements [Logger] and does nothing.
func (NopLogger) Debug(context.Context, string, map[string]string) {}

// Error implements [Logger] and does nothing.
func (NopLogger) Error(context.Context, string, error, map[string]string) {}

// SlogLogger adapts a standard library [slog.Logger] to [Logger].
type SlogLogger struct {
	// Logger is the destination logger. A nil value disables logging.
	Logger *slog.Logger
}

// Debug implements [Logger] by writing at slog debug level.
func (l SlogLogger) Debug(ctx context.Context, msg string, fields map[string]string) {
	if l.Logger == nil {
		return
	}
	l.Logger.DebugContext(ctx, msg, slogAttrs(fields)...)
}

// Error implements [Logger] by writing at slog error level.
func (l SlogLogger) Error(ctx context.Context, msg string, err error, fields map[string]string) {
	if l.Logger == nil {
		return
	}
	attrs := slogAttrs(fields)
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	l.Logger.ErrorContext(ctx, msg, attrs...)
}

// slogAttrs converts the flat field map into slog arguments.
func slogAttrs(fields map[string]string) []any {
	attrs := make([]any, 0, len(fields))
	for k, v := range fields {
		attrs = append(attrs, slog.String(k, v))
	}
	return attrs
}
