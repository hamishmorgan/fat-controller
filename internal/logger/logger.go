// Package logger provides context-based slog helpers for the CLI.
package logger

import (
	"context"
	"log/slog"
)

type contextKey struct{}

// NewContext returns a new context carrying the given logger.
func NewContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// From returns the logger stored in ctx, or slog.Default() if none.
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
